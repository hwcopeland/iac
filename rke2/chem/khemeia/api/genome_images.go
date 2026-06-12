package main

// genome_images.go holds the worker-image selection for GenomeJob CRs.
//
// Unlike the other CRD kinds (whose image is keyed solely by kind via
// crdImageMapping), a GenomeJob's worker image depends on spec.calculation —
// one GenomeJob kind fans out to different worker images per calculation. This
// mirrors how engineContainerImages in handlers_docking_v2.go selects a docking
// image from the per-job engine name rather than from the CRD kind.
//
// The GEN-12 reconcile path consumes genomeCalcImageFor() to resolve the worker
// image once it has read spec.calculation. See docs/tdd/khemeia-genomics-core.md
// section 5.1 for the authoritative calculation -> image table.

// genomeCalcImages maps a GenomeJob spec.calculation value to its worker image.
//
// Notes per core TDD section 5.1:
//   - "resolve" is the adapter-only stage. The cached/AlphaFold-hit path runs
//     controller-internal with no worker pod; an ESMFold GenomeJob is spawned
//     only on an AlphaFold miss. It therefore has no dedicated worker image and
//     is intentionally absent from this map (callers treat a miss as "internal").
//   - "pocket_proximity" reuses the existing p2rank image; "pgx_docking" reuses
//     the existing gnina image — neither introduces a new container.
//   - "esmfold" and "ddg_stability" point at images that may not exist yet;
//     genome route registration is gated behind GENOME_ENABLED, and the calc
//     endpoints are only enabled once their worker images are built.
var genomeCalcImages = map[string]string{
	"esmfold":          "zot.hwcopeland.net/chem/esmfold:latest",
	"ddg_stability":    "zot.hwcopeland.net/chem/ddg:latest",
	"pocket_proximity": "zot.hwcopeland.net/chem/p2rank:latest", // reuses existing image
	"pgx_docking":      "zot.hwcopeland.net/chem/gnina:latest",  // reuses existing image
}

// genomeCalcImageFor returns the worker image for a GenomeJob calculation and
// whether one is defined. A calculation with no entry (notably "resolve") is
// handled controller-internally rather than by a worker pod, so callers MUST
// check the boolean rather than assuming an image always exists.
func genomeCalcImageFor(calculation string) (string, bool) {
	image, ok := genomeCalcImages[calculation]
	return image, ok
}
