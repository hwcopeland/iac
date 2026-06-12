// genome_dispatch.go implements the missing GEN-11/GEN-12 dispatcher that turns
// accepted variant_calc_jobs rows into GenomeJob CRs.
//
// Background (core Decision B1): GenomeSubmit is the source of truth for the DB
// rows -- it writes the variant_jobs parent and one variant_calc_jobs child per
// accepted (variant × calculation). The CRDController is informer-driven over
// CRs that already exist; nothing converts the Pending rows into GenomeJob CRs.
// Without this dispatcher the pipeline is disconnected: rows sit Pending forever
// with cr_name NULL and no reconcile ever runs.
//
// Approach: submit-time create, mirroring the docking/advance path
// (handlers_crd.go HandleAdvance -> dynamicClient.Resource(gvr).Create). The rest
// of the platform mints CRs synchronously at the request edge rather than from a
// poll loop, so we do the same: the calc CR is created in the same handler that
// wrote the row, and its deterministic name is stamped back into
// variant_calc_jobs.cr_name so every cr_name-keyed join (setCalcJobStatus,
// rollupGroupStatus, the worker's `WHERE cr_name=$JOB_NAME`) lines up.
//
// Idempotency: the CR name is a deterministic function of (group, variant_key,
// calculation), so a retried submit converges on the same CR; AlreadyExists is
// tolerated. cr_name is stamped regardless so a partially-failed prior submit
// self-heals.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// genomeCalcJobCRName derives the deterministic GenomeJob CR name for a calc job.
// (group, variant_key, calculation) is the calc job's unique key (uq_calc_job),
// so hashing it gives a stable, DNS-1123-safe name that a retried submit
// converges on. The "gj-" prefix namespaces it away from the resolution-scoped
// esmfold child names ("esmfold-<rid>" / "esmfold-mut-<rid>").
func genomeCalcJobCRName(groupName, variantKey, calculation string) string {
	sum := sha256.Sum256([]byte(groupName + "\x00" + variantKey + "\x00" + calculation))
	return "gj-" + hex.EncodeToString(sum[:])[:16]
}

// genomeVariantSpecBlock projects a submitted VariantInput onto the CRD's
// spec.variant block. The CRD uses camelCase keys (proteinChange, uniprotAcc) --
// the inverse of variantInputFromCR (genome_reconcile.go) -- so empty fields are
// omitted to keep the CR minimal and the apiserver schema happy.
func genomeVariantSpecBlock(v VariantInput) map[string]interface{} {
	block := map[string]interface{}{"mode": v.Mode}
	put := func(key, val string) {
		if val != "" {
			block[key] = val
		}
	}
	put("gene", v.Gene)
	put("transcript", v.Transcript)
	put("proteinChange", v.ProteinChange)
	put("hgvs", v.HGVS)
	put("rsid", v.RSID)
	put("uniprotAcc", v.UniProtAcc)
	return block
}

// dispatchGenomeCalcJob mints one GenomeJob CR for an accepted (variant, calc)
// pair and stamps the CR name back onto its variant_calc_jobs row. It is the
// per-row half of the dispatcher invoked from GenomeSubmit.
//
// The CR is created Pending/gate:auto so reconcileGenomeJob (GEN-12) picks it up,
// runs the resolve stage, gates on any esmfold structure fold, then dispatches
// the calc worker. spec.calculation drives the two-stage lifecycle and the
// per-calc worker image (genomeCalcImageFor); spec.params is passed through
// verbatim for the worker. role/model/resolutionId (B2/B3 control fields) are NOT
// set here -- a top-level calc CR resolves its own variant and the resolve stage
// stamps resolution onto the esmfold CHILDREN it mints; setting them on the
// parent would mis-key the worker.
func (h *APIHandler) dispatchGenomeCalcJob(ctx context.Context, groupName string, v VariantInput, variantKey, calculation string, params json.RawMessage) error {
	gvr, ok := registeredCRDs["GenomeJob"]
	if !ok {
		return fmt.Errorf("GenomeJob CRD is not registered")
	}
	dyn := h.controller.dynamicClient
	if dyn == nil {
		return fmt.Errorf("dynamic client unavailable")
	}
	namespace := h.controller.namespace

	crName := genomeCalcJobCRName(groupName, variantKey, calculation)

	spec := map[string]interface{}{
		"calculation": calculation,
		"variant":     genomeVariantSpecBlock(v),
		"gate":        "auto",
		// computeClass is intentionally omitted: reconcile resolves the per-calc
		// default (genomeCalcComputeClass) when spec.computeClass is empty.
	}
	// Pass per-calc params through verbatim (already shape-validated at submit).
	if len(params) > 0 {
		var paramsObj map[string]interface{}
		if err := json.Unmarshal(params, &paramsObj); err != nil {
			return fmt.Errorf("calc %q params not a JSON object: %w", calculation, err)
		}
		spec["params"] = paramsObj
	}

	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", khemeiaGroup, khemeiaVersion),
			"kind":       "GenomeJob",
			"metadata": map[string]interface{}{
				"name":      crName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"khemeia.io/managed-by":  "genome-dispatch",
					"khemeia.io/group-name":  labelSafe(groupName),
					"khemeia.io/calculation": calculation,
				},
			},
			"spec": spec,
			"status": map[string]interface{}{
				"phase": "Pending",
			},
		},
	}

	_, err := dyn.Resource(gvr).Namespace(namespace).Create(ctx, cr, metav1.CreateOptions{})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("create GenomeJob CR %s: %w", crName, err)
	}

	// Stamp the CR name back onto the row so the cr_name-keyed joins line up.
	// Done even on AlreadyExists so a partially-failed prior submit self-heals.
	db := h.controller.firstDB()
	if db == nil {
		return fmt.Errorf("no database available to stamp cr_name")
	}
	if _, derr := db.ExecContext(ctx,
		`UPDATE variant_calc_jobs SET cr_name = ?
		 WHERE group_name = ? AND variant_key = ? AND calculation = ?`,
		crName, groupName, variantKey, calculation); derr != nil {
		return fmt.Errorf("stamp cr_name for %s/%s/%s: %w", groupName, variantKey, calculation, derr)
	}
	return nil
}

// labelSafe trims a value to the 63-char Kubernetes label-value limit so a long
// group name cannot make the CR creation fail label validation. The label is for
// observability only; the authoritative group linkage is the variant_calc_jobs row.
func labelSafe(v string) string {
	if len(v) > 63 {
		return v[:63]
	}
	return v
}

// dispatchGenomeGroup mints a GenomeJob CR for every accepted (variant × calc)
// pair in a submitted group and stamps each CR name back onto its row. A per-row
// failure is logged and the dispatch continues -- one un-mintable calc must not
// strand the rest of the batch (the row stays Pending with cr_name NULL and is
// picked up on a resubmit, exactly as it would have been before). Returns the
// number of CRs successfully dispatched.
func (h *APIHandler) dispatchGenomeGroup(ctx context.Context, groupName string, accepted []VariantInput, acceptedIDs, calculations []string, params map[string]json.RawMessage) int {
	dispatched := 0
	for i, v := range accepted {
		variantKey := acceptedIDs[i]
		for _, calc := range calculations {
			if err := h.dispatchGenomeCalcJob(ctx, groupName, v, variantKey, calc, params[calc]); err != nil {
				log.Printf("[genome] group %s: failed to dispatch CR for %s/%s: %v", groupName, variantKey, calc, err)
				continue
			}
			dispatched++
		}
	}
	log.Printf("[genome] group %s: dispatched %d/%d GenomeJob CRs", groupName, dispatched, len(accepted)*len(calculations))
	return dispatched
}
