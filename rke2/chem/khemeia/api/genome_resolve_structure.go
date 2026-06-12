// genome_resolve_structure.go: structure resolution for the variant adapter
// (core TDD §5.2 step 3). Given a resolved (acc, sequence) on a ResolvedVariant,
// it determines the structure source and, on an AlphaFold hit, downloads the
// model and stores it to the khemeia-structures bucket. On an AlphaFold miss it
// marks the variant for ESMFold fallback (structure_source=esmfold,
// NeedsStructureFold=true) WITHOUT folding -- the actual ESMFold run is GEN-20,
// minted by the GEN-12 reconcile, which inspects NeedsStructureFold.
package main

import (
	"bytes"
	"context"
	"fmt"
)

// BucketStructures is the Garage bucket for resolved WT/variant structures
// (core §5.5; created in Phase 0). Declared here (not s3.go, owned by a peer
// phase) to keep the adapter self-contained.
const BucketStructures = "khemeia-structures"

// resolveStructure sets rv.StructureSrc and rv.StructureS3 by trying AlphaFold
// DB first (the common path) and falling back to an ESMFold marker on a miss.
//
// On an explicit PDB override (rv.StructureSrc already == "pdb" with a key set
// by a caller) this is a no-op -- the operator pinned a structure. The adapter
// itself never sets "pdb"; that path exists for the rare override documented in
// core §5.2 and is honored if a caller pre-populates it.
func (rsv *Resolver) resolveStructure(ctx context.Context, s3 S3Client, rv *ResolvedVariant) error {
	if rv.StructureSrc == StructureSourcePDB && rv.StructureS3.Key != "" {
		return nil // operator-pinned PDB; nothing to fetch.
	}

	// The structure key is resolution-scoped so artifacts dedup with the cache.
	// rv.ResolutionID is now source-independent and is finalized by the caller
	// BEFORE this method runs, so the AlphaFold-hit and esmfold-fallback paths key
	// on the SAME id -- one substitution, one structure path, regardless of source.
	resKey := structureKey(rv.ResolutionID)

	pdb, plddt, ok, err := rsv.alphafold.FetchModel(ctx, rv.UniProtAcc)
	if err != nil {
		// Transport/5xx -> typed upstream error (batch continues per core R1).
		return err
	}

	if ok {
		if err := s3.PutArtifact(ctx, BucketStructures, resKey, bytes.NewReader(pdb), "chemical/x-pdb"); err != nil {
			return newResolveError(ECodeResolveUpstream,
				fmt.Sprintf("storing AlphaFold structure for %s to S3", rv.UniProtAcc), err)
		}
		rv.StructureSrc = StructureSourceAlphaFold
		rv.StructureS3 = ResolvedStructureS3{
			Bucket:      BucketStructures,
			Key:         resKey,
			PlddtGlobal: plddt,
		}
		rv.NeedsStructureFold = false
		return nil
	}

	// AlphaFold miss -> mark for ESMFold fallback. No structure is stored yet;
	// GEN-12 mints an esmfold GenomeJob as a parentJob and the worker (GEN-20)
	// writes resolve/{resolution_id}/structure.pdb + updates the row's structure_key /
	// plddt_global on completion. We record the intended key so the worker and
	// the consumer agree on the path.
	rv.StructureSrc = StructureSourceESMFold
	rv.StructureS3 = ResolvedStructureS3{
		Bucket:      BucketStructures,
		Key:         resKey,
		PlddtGlobal: nil, // unknown until the fold completes.
	}
	rv.NeedsStructureFold = true
	return nil
}

// structureKey builds the resolution-scoped key for a resolved WT structure
// (core §5.5: khemeia-structures/resolve/{resolution_id}/structure.pdb).
func structureKey(resolutionID string) string {
	return ArtifactKey("resolve", resolutionID, "structure", "pdb")
}
