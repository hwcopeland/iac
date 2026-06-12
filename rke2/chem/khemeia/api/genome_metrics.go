package main

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// genome_metrics.go declares the 13 OTel instruments that back the "Swamp"
// Grafana dashboard (platform TDD §5.3). They attach to the already-wired
// OTel→Prometheus reader (telemetry.go::initMetrics) and export under the
// existing khemeia-controller ServiceMonitor scrape.
//
// This file is DECLARATION + thin recording helpers only. Emission call sites
// live in the adapter/handlers (submit + resolution counters), the controller
// reconcile (lifecycle counters), and the result-writer drain (headline
// distributions) — wired by their respective issues.
//
// Naming: instruments are declared with dotted OTel names ("khemeia.genome.*");
// the Prometheus exporter sanitizes "." → "_" and appends "_total" to monotonic
// counters, yielding the contracted "khemeia_genome_*" / "khemeia_variant_*"
// metric names. Counters are named WITHOUT a trailing "_total" here so the
// exporter does not double-suffix it.

// The 13 instruments from platform TDD §5.3. Package-level so callers in this
// package (and a mirror in result-writer) can record without re-resolving them.
var (
	// 1. Variants submitted (REST submit handler). → khemeia_variants_submitted_total
	variantsSubmitted metric.Int64Counter

	// 2. Variant resolutions, labelled outcome + structure_source (adapter).
	//    → khemeia_variant_resolutions_total
	variantResolutions metric.Int64Counter

	// 3. Resolution cache hit/miss (adapter cache lookup).
	//    → khemeia_variant_resolution_cache_total
	variantResolutionCache metric.Int64Counter

	// 4. Resolution errors by typed code E_* (adapter).
	//    → khemeia_variant_resolution_errors_total
	variantResolutionErrors metric.Int64Counter

	// 5. Genome groups by status (controller group-status updater).
	//    → khemeia_genome_groups_total
	genomeGroups metric.Int64Counter

	// 6. Calc jobs by calculation + status (controller reconcile / result-writer).
	//    → khemeia_genome_calc_jobs_total
	genomeCalcJobs metric.Int64Counter

	// 7. Calc jobs in flight by calculation (controller reconcile).
	//    → khemeia_genome_calc_jobs_inflight
	genomeCalcJobsInflight metric.Int64UpDownCounter

	// 8. Per-calc job duration seconds, labelled calculation (controller).
	//    → khemeia_genome_calc_duration_seconds
	genomeCalcDuration metric.Float64Histogram

	// 9. ΔΔG-fold distribution (result-writer ddg_stability drain).
	//    → khemeia_genome_ddg_fold_kcal_mol
	genomeDdgFold metric.Float64Histogram

	// 10a. PGx ΔΔG-bind distribution (result-writer pgx_docking drain).
	//      → khemeia_genome_ddg_bind_kcal_mol
	genomeDdgBind metric.Float64Histogram

	// 10b. PGx fingerprint-delta tanimoto distribution (result-writer pgx_docking drain).
	//      → khemeia_genome_fp_delta_tanimoto
	genomeFpDeltaTanimoto metric.Float64Histogram

	// 11. Pocket-proximity hits by within_cutoff (result-writer pocket drain).
	//     → khemeia_genome_pocket_proximity_total
	genomePocketProximity metric.Int64Counter

	// 12. ESMFold pLDDT distribution (result-writer esmfold drain).
	//     → khemeia_genome_esmfold_plddt
	genomeEsmfoldPlddt metric.Float64Histogram
)

// Histogram bucket boundaries, taken verbatim from platform TDD §5.2 panels 9-13.
var (
	ddgFoldBuckets      = []float64{-3, -1, 0, 1, 2, 3, 5, 10}
	ddgBindBuckets      = []float64{-3, -1, 0, 1, 2, 3, 5, 10}
	fpDeltaBuckets      = []float64{0, 0.5, 0.7, 0.85, 0.95, 1.0}
	esmfoldPlddtBuckets = []float64{50, 70, 80, 90, 100}
	calcDurationBuckets = []float64{30, 60, 120, 300, 600, 1800, 3600, 7200, 21600, 86400}
)

// InitGenomeMetrics creates the 13 genome OTel instruments on the supplied meter
// (obtained from the global MeterProvider that telemetry.go::initMetrics wired to
// the Prometheus exporter). Call once at startup after initMetrics. Idempotency
// is the caller's responsibility — call exactly once.
func InitGenomeMetrics(meter metric.Meter) error {
	var err error

	if variantsSubmitted, err = meter.Int64Counter(
		"khemeia.variants.submitted",
		metric.WithDescription("Total variants accepted by the genome submit handler."),
	); err != nil {
		return fmt.Errorf("creating khemeia.variants.submitted: %w", err)
	}

	if variantResolutions, err = meter.Int64Counter(
		"khemeia.variant.resolutions",
		metric.WithDescription("Variant resolution outcomes by outcome and structure_source."),
	); err != nil {
		return fmt.Errorf("creating khemeia.variant.resolutions: %w", err)
	}

	if variantResolutionCache, err = meter.Int64Counter(
		"khemeia.variant.resolution.cache",
		metric.WithDescription("Variant resolution cache lookups by result (hit/miss)."),
	); err != nil {
		return fmt.Errorf("creating khemeia.variant.resolution.cache: %w", err)
	}

	if variantResolutionErrors, err = meter.Int64Counter(
		"khemeia.variant.resolution.errors",
		metric.WithDescription("Variant resolution errors by typed code (E_*)."),
	); err != nil {
		return fmt.Errorf("creating khemeia.variant.resolution.errors: %w", err)
	}

	if genomeGroups, err = meter.Int64Counter(
		"khemeia.genome.groups",
		metric.WithDescription("Genome job groups by terminal status."),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.groups: %w", err)
	}

	if genomeCalcJobs, err = meter.Int64Counter(
		"khemeia.genome.calc.jobs",
		metric.WithDescription("Genome calc jobs by calculation and status."),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.calc.jobs: %w", err)
	}

	if genomeCalcJobsInflight, err = meter.Int64UpDownCounter(
		"khemeia.genome.calc.jobs.inflight",
		metric.WithDescription("Genome calc jobs currently in flight by calculation."),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.calc.jobs.inflight: %w", err)
	}

	if genomeCalcDuration, err = meter.Float64Histogram(
		"khemeia.genome.calc.duration.seconds",
		metric.WithDescription("Per-calc genome job duration in seconds (start→terminal)."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(calcDurationBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.calc.duration.seconds: %w", err)
	}

	if genomeDdgFold, err = meter.Float64Histogram(
		"khemeia.genome.ddg.fold.kcal.mol",
		metric.WithDescription("ΔΔG of folding distribution (kcal/mol)."),
		metric.WithExplicitBucketBoundaries(ddgFoldBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.ddg.fold.kcal.mol: %w", err)
	}

	if genomeDdgBind, err = meter.Float64Histogram(
		"khemeia.genome.ddg.bind.kcal.mol",
		metric.WithDescription("PGx ΔΔG of binding distribution (kcal/mol, mut-wt)."),
		metric.WithExplicitBucketBoundaries(ddgBindBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.ddg.bind.kcal.mol: %w", err)
	}

	if genomeFpDeltaTanimoto, err = meter.Float64Histogram(
		"khemeia.genome.fp.delta.tanimoto",
		metric.WithDescription("PGx interaction-fingerprint delta tanimoto distribution (0..1)."),
		metric.WithExplicitBucketBoundaries(fpDeltaBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.fp.delta.tanimoto: %w", err)
	}

	if genomePocketProximity, err = meter.Int64Counter(
		"khemeia.genome.pocket.proximity",
		metric.WithDescription("Pocket-proximity results by within_cutoff (true/false)."),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.pocket.proximity: %w", err)
	}

	if genomeEsmfoldPlddt, err = meter.Float64Histogram(
		"khemeia.genome.esmfold.plddt",
		metric.WithDescription("ESMFold mean pLDDT distribution (0..100)."),
		metric.WithExplicitBucketBoundaries(esmfoldPlddtBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.esmfold.plddt: %w", err)
	}

	return nil
}

// --- Thin recording helpers (exported for callers in adapter/handlers/reconcile/
// result-writer). Each is nil-safe: if InitGenomeMetrics has not run (e.g. metrics
// init failed at startup), the helper is a no-op rather than a panic. ---

// RecordVariantSubmitted records n accepted variants at submit time (panel 5).
func RecordVariantSubmitted(ctx context.Context, n int) {
	if variantsSubmitted == nil {
		return
	}
	variantsSubmitted.Add(ctx, int64(n))
}

// RecordVariantResolution records one resolution outcome (panels 5,7).
// outcome ∈ {resolved, rejected}; structureSource ∈ {alphafold, esmfold, pdb, ""}.
func RecordVariantResolution(ctx context.Context, outcome, structureSource string) {
	if variantResolutions == nil {
		return
	}
	variantResolutions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
		attribute.String("structure_source", structureSource),
	))
}

// RecordResolutionCache records a cache lookup result (panel 6). result ∈ {hit, miss}.
func RecordResolutionCache(ctx context.Context, result string) {
	if variantResolutionCache == nil {
		return
	}
	variantResolutionCache.Add(ctx, 1, metric.WithAttributes(
		attribute.String("result", result),
	))
}

// RecordResolutionError records a typed resolution error (panel 8). code is an E_* string.
func RecordResolutionError(ctx context.Context, code string) {
	if variantResolutionErrors == nil {
		return
	}
	variantResolutionErrors.Add(ctx, 1, metric.WithAttributes(
		attribute.String("code", code),
	))
}

// RecordGenomeGroup records a genome group reaching a terminal status (panel 3).
func RecordGenomeGroup(ctx context.Context, status string) {
	if genomeGroups == nil {
		return
	}
	genomeGroups.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", status),
	))
}

// RecordCalcJob records a calc job status transition (panels 1,4).
func RecordCalcJob(ctx context.Context, calculation, status string) {
	if genomeCalcJobs == nil {
		return
	}
	genomeCalcJobs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("calculation", calculation),
		attribute.String("status", status),
	))
}

// IncCalcJobInflight / DecCalcJobInflight track in-flight calc jobs (panel 2).
func IncCalcJobInflight(ctx context.Context, calculation string) {
	if genomeCalcJobsInflight == nil {
		return
	}
	genomeCalcJobsInflight.Add(ctx, 1, metric.WithAttributes(
		attribute.String("calculation", calculation),
	))
}

func DecCalcJobInflight(ctx context.Context, calculation string) {
	if genomeCalcJobsInflight == nil {
		return
	}
	genomeCalcJobsInflight.Add(ctx, -1, metric.WithAttributes(
		attribute.String("calculation", calculation),
	))
}

// ObserveCalcDuration records a per-calc job duration in seconds (panel 13).
func ObserveCalcDuration(ctx context.Context, calculation string, seconds float64) {
	if genomeCalcDuration == nil {
		return
	}
	genomeCalcDuration.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("calculation", calculation),
	))
}

// ObserveDdgFold records a ΔΔG-fold value (panel 9, result-writer drain).
func ObserveDdgFold(ctx context.Context, v float64) {
	if genomeDdgFold == nil {
		return
	}
	genomeDdgFold.Record(ctx, v)
}

// ObserveDdgBind records a PGx ΔΔG-bind value (panel 10, result-writer drain).
func ObserveDdgBind(ctx context.Context, v float64) {
	if genomeDdgBind == nil {
		return
	}
	genomeDdgBind.Record(ctx, v)
}

// ObserveFpDeltaTanimoto records a fingerprint-delta tanimoto (panel 10, result-writer drain).
func ObserveFpDeltaTanimoto(ctx context.Context, v float64) {
	if genomeFpDeltaTanimoto == nil {
		return
	}
	genomeFpDeltaTanimoto.Record(ctx, v)
}

// RecordPocketProximity records a pocket-proximity result (panel 11, result-writer drain).
func RecordPocketProximity(ctx context.Context, withinCutoff bool) {
	if genomePocketProximity == nil {
		return
	}
	genomePocketProximity.Add(ctx, 1, metric.WithAttributes(
		attribute.Bool("within_cutoff", withinCutoff),
	))
}

// ObserveEsmfoldPlddt records an ESMFold mean pLDDT (panel 12, result-writer drain).
func ObserveEsmfoldPlddt(ctx context.Context, v float64) {
	if genomeEsmfoldPlddt == nil {
		return
	}
	genomeEsmfoldPlddt.Record(ctx, v)
}
