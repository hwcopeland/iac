// genome_reconcile.go implements the GenomeJob-specific reconcile loop for the
// Khemeia genomics structural-biophysics tooling layer (GEN-12).
//
// Where the generic CRD reconcile (crd_controller.go) drives a one-stage
// "Pending -> create Job -> monitor" lifecycle keyed only on phase, a GenomeJob
// is TWO-stage (core TDD §4, §5.2):
//
//	stage 1 (resolve):  ensure a ResolvedVariant exists for spec.variant. The
//	                    adapter (GEN-10 ResolveVariant) handles UniProt mapping,
//	                    WT validation, and structure resolution. On an AlphaFold
//	                    hit it stores the WT structure to S3 and returns
//	                    structure_source="alphafold". On an AlphaFold miss it
//	                    returns NeedsStructureFold=true / structure_source=
//	                    "esmfold" WITHOUT folding -- this reconcile then mints an
//	                    `esmfold` GenomeJob as a parentJob dependency and gates
//	                    the calc until that fold Succeeds.
//
//	stage 2 (dispatch): once a structure is available, dispatch the calc worker
//	                    Job. The worker image comes from genomeCalcImageFor()
//	                    (GEN-02) keyed on spec.calculation, and the compute class
//	                    comes from genomeCalcComputeClass() per core §5.1. This is
//	                    the generic createAndTrackJob/buildJobForCRD path, reached
//	                    only after the resolve stage marks the CR ready.
//
// CR phase transitions flow back into variant_calc_jobs (per-(variant,calc)
// status); the group-level variant_jobs.status rollup is advanced here (the
// controller, not the result-writer, owns group status -- core §5.6). Lifecycle
// metrics are emitted via the GEN-04 helpers at the matching transitions.
//
// This file owns only GenomeJob logic. It hooks into crd_controller.go at two
// surgical points: a kind branch at the top of reconcile(), and the worker-image
// resolution inside buildJobForCRD() (replacing the GEN-12 TODO guard).
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// genomeCalcComputeClass returns the default compute class for a GenomeJob
// calculation (core TDD §5.1). The GenomeJob kind default is "gpu"
// (defaultComputeClassForKind), but the real class is per-calculation: a
// destabilizing-free ddG run is CPU-bound, pocket detection is light CPU, while
// esmfold and pgx_docking need the GPU node. An explicit spec.computeClass
// always wins over this default (resolved in buildJobForCRD).
func genomeCalcComputeClass(calculation string) string {
	switch calculation {
	case "esmfold":
		return "gpu"
	case "ddg_stability":
		return "cpu-high-mem"
	case "pocket_proximity":
		return "cpu"
	case "pgx_docking":
		return "gpu"
	default:
		// "resolve" has no worker pod; fall back to the kind default.
		return defaultComputeClassForKind["GenomeJob"]
	}
}

// reconcileGenomeJob is the GenomeJob entrypoint, dispatched from reconcile()
// for kind=="GenomeJob". It runs the two-stage resolve-then-dispatch flow.
//
// The generic phase semantics still apply for Running/Failed/Succeeded (the calc
// worker Job is monitored/retried by the generic path). reconcileGenomeJob only
// owns the Pending phase, where it inserts the resolve stage ahead of dispatch.
func (c *CRDController) reconcileGenomeJob(u *unstructured.Unstructured) {
	name := u.GetName()
	phase := getPhase(u)

	switch phase {
	case "Pending":
		gate := getSpecString(u, "gate")
		if gate != "auto" {
			// gate:manual -- wait for the advance API, same as the generic path.
			return
		}
		c.reconcileGenomePending(u)

	case "Running":
		// The calc worker Job is a standard K8s Job; reuse the generic monitor.
		// On terminal transition it mirrors the calc-job DB row + group rollup.
		c.monitorRunningGenomeJob(u)

	case "Failed":
		// Reuse generic retry semantics, then mirror status to the DB.
		retryCount := getStatusInt(u, "retryCount")
		maxRetries := getSpecInt(u, "maxRetries", 3)
		retryPolicy := getSpecString(u, "retryPolicy")
		c.setCalcJobStatus(u, "Failed")
		c.rollupGroupStatus(u)
		if retryPolicy == "never" || retryCount >= maxRetries {
			return
		}
		log.Printf("[genome] GenomeJob/%s: Failed, retrying (%d/%d)", name, retryCount+1, maxRetries)
		c.retryJob(u, retryCount)

	case "Succeeded":
		// No action: the result-writer drains the worker payload and completes
		// the variant_calc_jobs row; the rollup runs on the Running->Succeeded
		// edge inside monitorRunningGenomeJob.
	}
}

// reconcileGenomePending runs stage 1 (resolve) and gates stage 2 (dispatch).
//
//  1. resolve calc:    if spec.calculation == "resolve", this CR IS the resolve
//     stage. Run ResolveVariant; on success mark Succeeded (the cached/AF-hit
//     path has no worker pod). On an AlphaFold miss, mint the esmfold parentJob
//     and stay Pending until it Succeeds.
//  2. calc calc:       resolve the variant first (idempotent, cache-backed), set
//     status.resolvedVariantRef/structureSource, then -- if the structure still
//     needs folding -- ensure an esmfold parentJob and wait. Once dependencies
//     are ready, hand off to the generic createAndTrackJob (stage 2 dispatch).
func (c *CRDController) reconcileGenomePending(u *unstructured.Unstructured) {
	name := u.GetName()
	calculation := getSpecString(u, "calculation")
	if calculation == "" {
		c.failGenomeJob(u, "spec.calculation is required")
		return
	}

	ctx := context.Background()

	// Mark the owning calc-job row as Resolving while stage 1 runs.
	c.setCalcJobStatus(u, "Resolving")
	c.rollupGroupStatus(u)

	// Stage 1: resolve the variant (cache-backed; AlphaFold fetch + S3 store and
	// the esmfold-fallback marker are handled inside ResolveVariant / the adapter
	// structure stage). A typed ResolveError is mapped to a frozen E_* code.
	in, err := variantInputFromCR(u)
	if err != nil {
		c.failGenomeJobTyped(u, ECodeParamsInvalid, err.Error())
		return
	}

	rv, err := ResolveVariant(ctx, c.db, c.s3Client, in)
	if err != nil {
		code, ok := ResolveErrorCode(err)
		if !ok {
			code = ECodeResolveUpstream
		}
		c.failGenomeJobTyped(u, code, err.Error())
		return
	}

	// Persist the resolution handle onto the CR status so calc stages and the
	// worker (via env) can find it; also mirror it to the calc-job row.
	c.setGenomeResolution(u, rv)
	c.setCalcJobResolution(u, rv.ResolutionID)

	// The `resolve` calculation is the adapter-only stage: there is no worker
	// pod for the cached / AlphaFold-hit path. On an AlphaFold miss it mints an
	// esmfold child and gates on it via areDependenciesReady -- the resolve CR is
	// only "Succeeded" once the structure is GUARANTEED to exist (AlphaFold hit,
	// or the esmfold fold child has reached phase Succeeded), never merely once
	// the cache row exists.
	if calculation == "resolve" {
		if rv.NeedsStructureFold {
			c.ensureESMFoldParent(u, rv)
		}
		if !c.areDependenciesReady(u) {
			log.Printf("[genome] GenomeJob/%s (resolve): waiting on esmfold parentJob (structure fold)", name)
			return
		}
		c.succeedGenomeJob(u, "variant resolved (structure available)")
		c.setCalcJobStatus(u, "Completed")
		c.rollupGroupStatus(u)
		return
	}

	// Stage 2 gating, part A -- WT structure availability. If the WT structure is
	// not yet folded (AlphaFold miss), ensure a WT-fold esmfold parentJob and wait.
	if rv.NeedsStructureFold {
		c.ensureESMFoldParent(u, rv)
	}

	// Stage 2 gating, part B -- mutant structure availability. Calcs that diff WT
	// against the MUTANT structure (today: pgx_docking) consume
	// khemeia-structures/esmfold/{rid}/mut.pdb, which only the role-B esmfold calc
	// (MODEL=wt_and_mut) produces. AlphaFold supplies WT only and the WT-fallback
	// esmfold child (role A) folds WT only, so neither guarantees mut.pdb. Mint a
	// DISTINCT, deterministic role-B esmfold child as this CR's parentJob and gate
	// on it so the pgx worker never dispatches before mut.pdb exists (core §5.5,
	// workers §7.4; closes the GEN-40 E_STRUCTURE_FOLD_FAILED race).
	if calcNeedsMutantStructure(calculation) {
		c.ensureMutantESMFoldParent(u, rv)
	}

	if !c.areDependenciesReady(u) {
		log.Printf("[genome] GenomeJob/%s (%s): waiting on esmfold parentJob (structure fold)", name, calculation)
		return
	}

	// Dependencies ready (or never needed) -> dispatch the calc worker Job. The
	// generic path resolves the image via genomeCalcImageFor in buildJobForCRD.
	log.Printf("[genome] GenomeJob/%s (%s): structure ready -> dispatching calc worker", name, calculation)
	IncCalcJobInflight(ctx, calculation)
	RecordCalcJob(ctx, calculation, "Running")
	c.setCalcJobStatus(u, "Running")
	c.createAndTrackJob(u)
	c.rollupGroupStatus(u)
}

// monitorRunningGenomeJob watches the calc worker Job and, on a terminal edge,
// mirrors the result into variant_calc_jobs and advances the group rollup. It
// delegates the K8s Job status read to the generic monitor by re-reading the Job
// directly so it can also emit genome metrics and DB writes.
func (c *CRDController) monitorRunningGenomeJob(u *unstructured.Unstructured) {
	name := u.GetName()
	calculation := getSpecString(u, "calculation")
	jobName := crdJobName("GenomeJob", name)
	ctx := context.Background()

	job, err := c.jobClient.Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return // Job may not exist yet or was GC'd; wait for next reconcile.
	}

	switch {
	case job.Status.Succeeded > 0:
		log.Printf("[genome] GenomeJob/%s (%s): calc Job succeeded", name, calculation)
		c.updateCRDStatus(u, "Succeeded", "calc worker Job completed successfully")
		DecCalcJobInflight(ctx, calculation)
		RecordCalcJob(ctx, calculation, "Completed")
		c.observeCalcDurationFromCR(ctx, u, calculation)
		// The result-writer drains the worker payload and flips the calc-job row
		// to Completed; we only advance the group rollup here.
		c.rollupGroupStatus(u)

	case job.Status.Failed > 0:
		log.Printf("[genome] GenomeJob/%s (%s): calc Job failed", name, calculation)
		c.updateCRDStatus(u, "Failed", "calc worker Job failed")
		DecCalcJobInflight(ctx, calculation)
		RecordCalcJob(ctx, calculation, "Failed")
		c.observeCalcDurationFromCR(ctx, u, calculation)
		c.setCalcJobStatus(u, "Failed")
		c.rollupGroupStatus(u)
	}
	// Still active -> nothing to do; next reconcile re-checks.
}

// --- ESMFold parentJob minting (structure-fold dependency handoff) ---
//
// There are TWO distinct esmfold sub-roles (workers TDD §4.0), keyed by the
// worker's ROLE/MODEL env (fold.py §4.2 step 1 reads CALCULATION + ROLE + MODEL):
//
//	role A (resolve_fallback, MODEL=wt_only): folds the WT only and writes
//	  khemeia-structures/resolve/{rid}/structure.pdb. Minted on an AlphaFold miss
//	  during WT resolution so a WT structure exists at all. Does NOT produce mut.pdb.
//
//	role B (calc, MODEL=wt_and_mut): folds WT AND mutant and writes
//	  khemeia-structures/esmfold/{rid}/mut.pdb (+ wt.pdb). This is the ONLY producer
//	  of the mutant structure that pgx_docking (a WT-vs-mut diff) requires.
//
// The two roles produce different S3 artifacts, so they are DISTINCT deterministic
// children (esmfold-<rid> vs esmfold-mut-<rid>) rather than one child whose role is
// flipped in place -- that keeps each idempotent (AlreadyExists tolerated) and avoids
// mutating an already-running WT-fallback child. Both are resolution-scoped so repeated
// reconciles / multiple consumers of the same resolution converge on one fold job each.

// foldRole describes an esmfold sub-role: its deterministic child-name function,
// the worker ROLE/MODEL env it must run under, and the genome-role label.
type foldRole struct {
	role       string // worker ROLE env (fold.py §4.2)
	model      string // worker MODEL env (fold.py §4.2)
	label      string // khemeia.io/genome-role metadata label
	nameFor    func(resolutionID string) string
	logContext string
}

var (
	// foldRoleWT is the AlphaFold-miss WT fallback (role A): WT only.
	foldRoleWT = foldRole{
		role:       "resolve_fallback",
		model:      "wt_only",
		label:      "resolve-fallback",
		nameFor:    esmfoldJobName,
		logContext: "AlphaFold miss (WT fold)",
	}
	// foldRoleMutant is the role-B calc fold that produces mut.pdb (role B): WT+mut.
	foldRoleMutant = foldRole{
		role:       "calc",
		model:      "wt_and_mut",
		label:      "mutant-fold",
		nameFor:    esmfoldMutantJobName,
		logContext: "mutant structure dependency",
	}
)

// calcNeedsMutantStructure reports whether a calculation diffs WT against the
// MUTANT structure and therefore requires khemeia-structures/esmfold/{rid}/mut.pdb
// (workers §7.4). pgx_docking is the only such calc today; future mutant-consuming
// calcs are added here and inherit the role-B fold gating for free.
func calcNeedsMutantStructure(calculation string) bool {
	switch calculation {
	case "pgx_docking":
		return true
	default:
		return false
	}
}

// ensureESMFoldParent mints the WT-fallback (role A) esmfold child as the parentJob
// for the current CR. This is the AlphaFold-miss fallback: the resolved WT structure
// does not exist yet, so a role-A esmfold child folds WT; the current CR gates on it
// via areDependenciesReady before dispatching. Deterministic + idempotent.
func (c *CRDController) ensureESMFoldParent(u *unstructured.Unstructured, rv *ResolvedVariant) {
	c.ensureESMFoldParentForRole(u, rv, foldRoleWT)
}

// ensureMutantESMFoldParent mints the role-B esmfold child (MODEL=wt_and_mut) that
// produces mut.pdb, and wires it as the current CR's parentJob so a mutant-consuming
// calc (pgx_docking) cannot dispatch until the mutant fold has Succeeded. This is the
// GEN-40 fix: without it a pgx_docking GenomeJob could dispatch before any mut.pdb
// exists and the worker would fail fast with E_STRUCTURE_FOLD_FAILED.
func (c *CRDController) ensureMutantESMFoldParent(u *unstructured.Unstructured, rv *ResolvedVariant) {
	c.ensureESMFoldParentForRole(u, rv, foldRoleMutant)
}

// ensureESMFoldParentForRole is the shared minting path for both fold roles. It
// creates the deterministic role-specific esmfold child (tolerating AlreadyExists so
// concurrent reconciles / multiple consumers of the same resolution converge) and
// wires it as this CR's parentJob so areDependenciesReady gates dispatch on the fold
// reaching phase Succeeded.
func (c *CRDController) ensureESMFoldParentForRole(u *unstructured.Unstructured, rv *ResolvedVariant, fr foldRole) {
	name := u.GetName()

	// Idempotency: if this CR already has an esmfold parentJob wired, leave it.
	// A CR needs at most one structure-fold dependency: the WT-fallback path and
	// the mutant-fold path are mutually exclusive per calc (resolve needs only WT;
	// pgx_docking needs the mutant fold, whose role-B child folds WT too).
	if existing := c.parentJobName(u); existing != "" {
		return
	}

	// Do not mint a fold child for an esmfold CR itself (it would self-cycle).
	if getSpecString(u, "calculation") == "esmfold" {
		return
	}

	childName := fr.nameFor(rv.ResolutionID)

	ctx := context.Background()
	if err := c.createESMFoldChild(ctx, u, rv, childName, fr); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to mint esmfold parentJob %s (%s): %v", name, childName, fr.label, err)
		return
	}

	// Wire the parentJob dependency onto this CR's spec so areDependenciesReady
	// gates the calc until the fold Succeeds.
	c.setParentJob(u, "GenomeJob", childName)
	log.Printf("[genome] GenomeJob/%s: minted esmfold parentJob %s (%s)", name, childName, fr.logContext)
}

// createESMFoldChild creates the esmfold GenomeJob CR for the given fold role over
// the resolved variant. It mirrors the variant block + resolvedVariantRef so the
// worker (GEN-20) can fetch the sequence, and it sets SCALAR role/model spec fields
// so buildCRDJobEnv propagates them as ROLE/MODEL env -- the exact keys fold.py
// branches on to pick role A (WT-only resolve fallback) vs role B (wt_and_mut calc,
// which writes esmfold/{rid}/mut.pdb). Without these, the child would default and a
// WT-fallback child could be indistinguishable from the mutant fold.
func (c *CRDController) createESMFoldChild(ctx context.Context, u *unstructured.Unstructured, rv *ResolvedVariant, childName string, fr foldRole) error {
	gvr := registeredCRDs["GenomeJob"]

	// Copy the parent's variant block verbatim so the fold runs over the same
	// substitution coordinates.
	variant, _ := getSpec(u)["variant"].(map[string]interface{})

	child := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", khemeiaGroup, khemeiaVersion),
			"kind":       "GenomeJob",
			"metadata": map[string]interface{}{
				"name":      childName,
				"namespace": c.namespace,
				"labels": map[string]interface{}{
					"khemeia.io/managed-by":    "crd-controller",
					"khemeia.io/genome-role":   fr.label,
					"khemeia.io/resolution-id": rv.ResolutionID,
					"khemeia.io/parent-of":     u.GetName(),
				},
			},
			"spec": map[string]interface{}{
				"calculation":        "esmfold",
				"variant":            variant,
				"resolvedVariantRef": rv.ResolutionID,
				"computeClass":       genomeCalcComputeClass("esmfold"),
				"gate":               "auto",
				// Scalar fields -> ROLE / MODEL / RESOLUTION_ID env via buildCRDJobEnv;
				// fold.py keys role A vs role B off ROLE/MODEL (workers §4.0, §4.2) and
				// reads RESOLUTION_ID to find its resolution. resolutionId is REQUIRED on
				// the WT-fallback child: it has no variant_calc_jobs row to recover the
				// id from, so without this scalar the worker dies E_RESOLVE_UPSTREAM
				// (resolvedVariantRef maps to RESOLVEDVARIANTREF, which fold.py ignores).
				"resolutionId": rv.ResolutionID,
				"role":         fr.role,
				"model":        fr.model,
			},
			"status": map[string]interface{}{
				"phase":              "Pending",
				"resolvedVariantRef": rv.ResolutionID,
				"structureSource":    StructureSourceESMFold,
			},
		},
	}

	_, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		Create(ctx, child, metav1.CreateOptions{})
	if err != nil && !isAlreadyExists(err) {
		return err
	}
	return nil
}

// esmfoldJobName derives the deterministic role-A (WT fallback) esmfold child CR name
// from a resolution_id, so repeated AlphaFold-miss resolves converge on one fold job.
// resolution_id is "rv-<sha256-16>"; the resulting name is DNS-1123 safe.
func esmfoldJobName(resolutionID string) string {
	return "esmfold-" + resolutionID
}

// esmfoldMutantJobName derives the deterministic role-B (wt_and_mut) esmfold child CR
// name. It is DISTINCT from esmfoldJobName so the mutant fold (which writes
// esmfold/{rid}/mut.pdb) never collides with the WT-only fallback child; multiple
// mutant-consuming calcs for the same resolution converge on this one fold job.
func esmfoldMutantJobName(resolutionID string) string {
	return "esmfold-mut-" + resolutionID
}

// --- variant_calc_jobs / variant_jobs DB mirroring ---

// setCalcJobStatus updates the owning variant_calc_jobs row (keyed by cr_name)
// to a new status, stamping started_at/completed_at at the matching transitions.
// A missing row is logged and ignored (GEN-11 owns row creation; a CR may briefly
// exist before its row is committed).
func (c *CRDController) setCalcJobStatus(u *unstructured.Unstructured, status string) {
	crName := u.GetName()
	ctx := context.Background()

	var query string
	switch status {
	case "Running":
		query = `UPDATE variant_calc_jobs SET status = ?, started_at = COALESCE(started_at, NOW()) WHERE cr_name = ?`
	case "Completed", "Failed", "Skipped":
		query = `UPDATE variant_calc_jobs SET status = ?, completed_at = NOW() WHERE cr_name = ?`
	default: // Pending, Resolving
		query = `UPDATE variant_calc_jobs SET status = ? WHERE cr_name = ?`
	}

	if _, err := c.db.ExecContext(ctx, query, status, crName); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to set calc-job status %s: %v", crName, status, err)
	}
}

// setCalcJobResolution records the resolution_id on the owning calc-job row so
// the worker and result-writer can join back to the resolution cache.
func (c *CRDController) setCalcJobResolution(u *unstructured.Unstructured, resolutionID string) {
	crName := u.GetName()
	if _, err := c.db.ExecContext(context.Background(),
		`UPDATE variant_calc_jobs SET resolution_id = ? WHERE cr_name = ?`,
		resolutionID, crName); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to set calc-job resolution_id: %v", crName, err)
	}
}

// failGenomeJob marks the CR Failed and mirrors the calc-job row + rollup with a
// generic (non-typed) error message.
func (c *CRDController) failGenomeJob(u *unstructured.Unstructured, msg string) {
	c.failGenomeJobTyped(u, "", msg)
}

// failGenomeJobTyped marks the CR Failed, writes the typed E_* code (if any) into
// variant_calc_jobs.error_output, records the resolution-error metric, and rolls
// up the group. The CR phase enum has no "Skipped", so a resolution-time rejection
// surfaces as CR Failed while the calc-job row distinguishes Failed vs Skipped.
func (c *CRDController) failGenomeJobTyped(u *unstructured.Unstructured, code, msg string) {
	name := u.GetName()
	calculation := getSpecString(u, "calculation")
	ctx := context.Background()

	detail := msg
	if code != "" {
		detail = code + ": " + msg
		RecordResolutionError(ctx, code)
	}

	c.updateCRDStatus(u, "Failed", detail)
	if _, err := c.db.ExecContext(ctx,
		`UPDATE variant_calc_jobs SET status = 'Failed', error_output = ?, completed_at = NOW() WHERE cr_name = ?`,
		detail, name); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to write calc-job failure: %v", name, err)
	}
	if calculation != "" {
		RecordCalcJob(ctx, calculation, "Failed")
	}
	c.rollupGroupStatus(u)
	log.Printf("[genome] GenomeJob/%s (%s): FAILED: %s", name, calculation, detail)
}

// succeedGenomeJob marks the CR Succeeded (used for the worker-less resolve
// stage; calc CRs reach Succeeded via the Job monitor).
func (c *CRDController) succeedGenomeJob(u *unstructured.Unstructured, msg string) {
	c.updateCRDStatus(u, "Succeeded", msg)
}

// rollupGroupStatus advances the parent variant_jobs.status from the aggregate
// state of its variant_calc_jobs children (core §5.6: the controller, not the
// result-writer, owns group status). It is idempotent and safe to call on every
// reconcile edge.
//
// Rollup rules:
//   - any child still Pending/Resolving/Running  -> group Running
//   - all children terminal, >=1 Failed          -> group Failed
//   - all children terminal, none Failed         -> group Completed
//   - no children yet                            -> leave as-is
func (c *CRDController) rollupGroupStatus(u *unstructured.Unstructured) {
	groupName := c.groupNameForCR(u)
	if groupName == "" {
		return
	}
	ctx := context.Background()

	var total, pending, failed, skipped int
	row := c.db.QueryRowContext(ctx,
		`SELECT
		   COUNT(*),
		   COUNT(*) FILTER (WHERE status IN ('Pending','Resolving','Running')),
		   COUNT(*) FILTER (WHERE status = 'Failed'),
		   COUNT(*) FILTER (WHERE status = 'Skipped')
		 FROM variant_calc_jobs WHERE group_name = ?`, groupName)
	if err := row.Scan(&total, &pending, &failed, &skipped); err != nil {
		log.Printf("[genome] group %s: rollup query failed: %v", groupName, err)
		return
	}
	if total == 0 {
		return
	}

	var newStatus string
	switch {
	case pending > 0:
		newStatus = "Running"
	case failed > 0:
		newStatus = "Failed"
	default:
		newStatus = "Completed"
	}

	// Stamp started_at on first move to Running, completed_at on terminal; the
	// CHECK constraint guards the status values. Only emit the group metric on a
	// genuine terminal transition (status actually changed to terminal).
	var query string
	switch newStatus {
	case "Running":
		query = `UPDATE variant_jobs
		         SET status = ?, started_at = COALESCE(started_at, NOW())
		         WHERE group_name = ? AND status <> ?`
	default: // Completed | Failed
		query = `UPDATE variant_jobs
		         SET status = ?, completed_at = NOW()
		         WHERE group_name = ? AND status NOT IN ('Completed','Failed')`
	}

	res, err := c.db.ExecContext(ctx, query, newStatus, groupName, newStatus)
	if err != nil {
		log.Printf("[genome] group %s: rollup update failed: %v", groupName, err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 && (newStatus == "Completed" || newStatus == "Failed") {
		RecordGenomeGroup(ctx, newStatus)
		log.Printf("[genome] group %s -> %s (%d calc jobs, %d failed)", groupName, newStatus, total, failed)
	}
}

// groupNameForCR resolves the group_name for a GenomeJob CR by looking up its
// owning variant_calc_jobs row (keyed by cr_name). The esmfold fallback child
// has no calc-job row of its own, so this returns "" for it (no rollup needed).
func (c *CRDController) groupNameForCR(u *unstructured.Unstructured) string {
	var groupName string
	err := c.db.QueryRowContext(context.Background(),
		`SELECT group_name FROM variant_calc_jobs WHERE cr_name = ? LIMIT 1`, u.GetName()).
		Scan(&groupName)
	if err != nil {
		return ""
	}
	return groupName
}

// --- CR status / spec mutation helpers (unstructured) ---

// setGenomeResolution writes the resolved-variant handle onto the CR status:
// resolvedVariantRef + structureSource (core §5.1 status fields).
func (c *CRDController) setGenomeResolution(u *unstructured.Unstructured, rv *ResolvedVariant) {
	gvr := registeredCRDs["GenomeJob"]
	ctx := context.Background()

	latest, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		Get(ctx, u.GetName(), metav1.GetOptions{})
	if err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to get latest for resolution update: %v", u.GetName(), err)
		return
	}
	status := getStatusMap(latest)
	status["resolvedVariantRef"] = rv.ResolutionID
	status["structureSource"] = rv.StructureSrc
	latest.Object["status"] = status

	if _, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		UpdateStatus(ctx, latest, metav1.UpdateOptions{}); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to write resolution to status: %v", u.GetName(), err)
		return
	}
	// Reflect locally so subsequent same-pass logic sees the update.
	u.Object["status"] = status
}

// setParentJob wires a parentJob dependency onto the CR spec (consumed by
// areDependenciesReady). Mutates the spec subresource via a plain Update.
func (c *CRDController) setParentJob(u *unstructured.Unstructured, kind, name string) {
	gvr := registeredCRDs["GenomeJob"]
	ctx := context.Background()

	latest, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		Get(ctx, u.GetName(), metav1.GetOptions{})
	if err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to get latest for parentJob wiring: %v", u.GetName(), err)
		return
	}
	spec, _ := latest.Object["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
	}
	spec["parentJob"] = map[string]interface{}{"kind": kind, "name": name}
	latest.Object["spec"] = spec

	if _, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		Update(ctx, latest, metav1.UpdateOptions{}); err != nil {
		log.Printf("[genome] GenomeJob/%s: failed to wire parentJob: %v", u.GetName(), err)
		return
	}
	u.Object["spec"] = spec
}

// parentJobName returns the wired parentJob name from the CR spec, or "".
func (c *CRDController) parentJobName(u *unstructured.Unstructured) string {
	spec := getSpec(u)
	parent, ok := spec["parentJob"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := parent["name"].(string)
	return name
}

// observeCalcDurationFromCR records the per-calc job duration (start->terminal)
// from the CR status.startTime, in seconds (core §5.3 panel 13 distribution).
func (c *CRDController) observeCalcDurationFromCR(ctx context.Context, u *unstructured.Unstructured, calculation string) {
	status, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		return
	}
	startStr, _ := status["startTime"].(string)
	if startStr == "" {
		return
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return
	}
	ObserveCalcDuration(ctx, calculation, time.Since(start).Seconds())
}

// --- variant input reconstruction from a CR ---

// variantInputFromCR projects the GenomeJob spec.variant block (core §5.1) onto
// the adapter's VariantInput. The CRD uses camelCase JSON keys (proteinChange,
// uniprotAcc); the adapter struct uses snake_case JSON tags, so we read the
// unstructured map fields explicitly rather than round-tripping JSON.
func variantInputFromCR(u *unstructured.Unstructured) (VariantInput, error) {
	spec := getSpec(u)
	variant, ok := spec["variant"].(map[string]interface{})
	if !ok {
		return VariantInput{}, fmt.Errorf("spec.variant is missing or not an object")
	}

	str := func(k string) string {
		s, _ := variant[k].(string)
		return s
	}

	in := VariantInput{
		Mode:          str("mode"),
		Gene:          str("gene"),
		Transcript:    str("transcript"),
		ProteinChange: str("proteinChange"),
		HGVS:          str("hgvs"),
		RSID:          str("rsid"),
		UniProtAcc:    str("uniprotAcc"),
	}
	if in.Mode == "" {
		return VariantInput{}, fmt.Errorf("spec.variant.mode is required")
	}
	return in, nil
}

// --- small utilities ---

// isAlreadyExists reports whether err is a Kubernetes AlreadyExists error,
// tolerated when minting the deterministic esmfold child so concurrent reconciles
// of the same AlphaFold-miss variant converge on one fold job.
func isAlreadyExists(err error) bool {
	return k8serrors.IsAlreadyExists(err)
}
