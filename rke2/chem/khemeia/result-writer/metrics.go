// metrics.go declares the result-writer's OWN OTel meter and the five genome
// "distribution" instruments from platform TDD §5.3 (panels 9-12). The
// result-writer is a SEPARATE binary/package from the api controller, so it
// CANNOT import the api package's instrument vars — it must declare its own
// copies under identical metric names so both processes contribute to the same
// Prometheus series (scraped per-pod; aggregated in PromQL).
//
// Per platform TDD §5.3, these headline distributions are recorded at the
// result-writer drain choke-point (the single place every typed result lands),
// NOT in the four workers. The lifecycle counters (1-4,13) and the submit/
// resolution counters live in the controller/adapter (api package) and are out
// of scope for this binary.
//
// Naming: instruments are declared with dotted OTel names ("khemeia.genome.*");
// the Prometheus exporter sanitizes "." -> "_" and appends "_total" to monotonic
// counters, yielding the contracted "khemeia_genome_*" metric names. The counter
// is declared WITHOUT a trailing "_total" so the exporter does not double-suffix.
// This mirrors api/genome_metrics.go exactly so the two binaries' series align.
package main

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// The five genome distribution instruments from platform TDD §5.3 that are
// emitted at result-writer drain time. Package-level so processGenomeCalc can
// record without re-resolving them.
var (
	// 9. ΔΔG-fold distribution (ddg_stability drain). → khemeia_genome_ddg_fold_kcal_mol
	genomeDdgFold metric.Float64Histogram

	// 10a. PGx ΔΔG-bind distribution (pgx_docking drain). → khemeia_genome_ddg_bind_kcal_mol
	genomeDdgBind metric.Float64Histogram

	// 10b. PGx fingerprint-delta tanimoto (pgx_docking drain). → khemeia_genome_fp_delta_tanimoto
	genomeFpDeltaTanimoto metric.Float64Histogram

	// 11. Pocket-proximity hits by within_cutoff (pocket_proximity drain).
	//     → khemeia_genome_pocket_proximity_total
	genomePocketProximity metric.Int64Counter

	// 12. ESMFold mean pLDDT distribution (esmfold drain). → khemeia_genome_esmfold_plddt
	genomeEsmfoldPlddt metric.Float64Histogram
)

// Histogram bucket boundaries, taken verbatim from api/genome_metrics.go (which
// in turn takes them from platform TDD §5.2 panels 9-13) so the two binaries'
// histograms share identical buckets and aggregate cleanly in Prometheus.
var (
	ddgFoldBuckets      = []float64{-3, -1, 0, 1, 2, 3, 5, 10}
	ddgBindBuckets      = []float64{-3, -1, 0, 1, 2, 3, 5, 10}
	fpDeltaBuckets      = []float64{0, 0.5, 0.7, 0.85, 0.95, 1.0}
	esmfoldPlddtBuckets = []float64{50, 70, 80, 90, 100}
)

// initMetrics wires an OTel→Prometheus reader, registers it as the global
// MeterProvider, and creates the five genome distribution instruments on a meter
// scoped to "khemeia-result-writer". It returns a shutdown func and any error.
//
// On error the caller should log-and-continue: the recording helpers below are
// nil-safe, so a metrics-init failure degrades to no-op metrics rather than
// taking down the drain loop.
func initMetrics(ctx context.Context) (func(context.Context) error, error) {
	exp, err := prometheus.New()
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("creating prometheus exporter: %w", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("khemeia-result-writer"),
	)
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exp),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(provider)

	meter := provider.Meter("khemeia-result-writer")
	if err := initGenomeInstruments(meter); err != nil {
		return provider.Shutdown, err
	}

	return provider.Shutdown, nil
}

// initGenomeInstruments creates the five distribution instruments on the meter.
func initGenomeInstruments(meter metric.Meter) error {
	var err error

	if genomeDdgFold, err = meter.Float64Histogram(
		"khemeia.genome.ddg.fold.kcal.mol",
		metric.WithDescription("ΔΔG of folding distribution (kcal/mol), recorded at result-writer drain."),
		metric.WithExplicitBucketBoundaries(ddgFoldBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.ddg.fold.kcal.mol: %w", err)
	}

	if genomeDdgBind, err = meter.Float64Histogram(
		"khemeia.genome.ddg.bind.kcal.mol",
		metric.WithDescription("PGx ΔΔG of binding distribution (kcal/mol, mut-wt), recorded at result-writer drain."),
		metric.WithExplicitBucketBoundaries(ddgBindBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.ddg.bind.kcal.mol: %w", err)
	}

	if genomeFpDeltaTanimoto, err = meter.Float64Histogram(
		"khemeia.genome.fp.delta.tanimoto",
		metric.WithDescription("PGx interaction-fingerprint delta tanimoto (0..1), recorded at result-writer drain."),
		metric.WithExplicitBucketBoundaries(fpDeltaBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.fp.delta.tanimoto: %w", err)
	}

	if genomePocketProximity, err = meter.Int64Counter(
		"khemeia.genome.pocket.proximity",
		metric.WithDescription("Pocket-proximity results by within_cutoff (true/false), recorded at result-writer drain."),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.pocket.proximity: %w", err)
	}

	if genomeEsmfoldPlddt, err = meter.Float64Histogram(
		"khemeia.genome.esmfold.plddt",
		metric.WithDescription("ESMFold mean pLDDT distribution (0..100), recorded at result-writer drain."),
		metric.WithExplicitBucketBoundaries(esmfoldPlddtBuckets...),
	); err != nil {
		return fmt.Errorf("creating khemeia.genome.esmfold.plddt: %w", err)
	}

	return nil
}

// --- Thin, nil-safe recording helpers. If initMetrics failed at startup the
// instruments are nil and each helper is a no-op rather than a panic. ---

// observeDdgFold records a ΔΔG-fold value (panel 9, ddg_stability drain).
func observeDdgFold(ctx context.Context, v float64) {
	if genomeDdgFold == nil {
		return
	}
	genomeDdgFold.Record(ctx, v)
}

// observeDdgBind records a PGx ΔΔG-bind value (panel 10, pgx_docking drain).
func observeDdgBind(ctx context.Context, v float64) {
	if genomeDdgBind == nil {
		return
	}
	genomeDdgBind.Record(ctx, v)
}

// observeFpDeltaTanimoto records a fingerprint-delta tanimoto (panel 10, pgx_docking drain).
func observeFpDeltaTanimoto(ctx context.Context, v float64) {
	if genomeFpDeltaTanimoto == nil {
		return
	}
	genomeFpDeltaTanimoto.Record(ctx, v)
}

// recordPocketProximity records a pocket-proximity result (panel 11, pocket_proximity drain).
func recordPocketProximity(ctx context.Context, withinCutoff bool) {
	if genomePocketProximity == nil {
		return
	}
	genomePocketProximity.Add(ctx, 1, metric.WithAttributes(
		attribute.Bool("within_cutoff", withinCutoff),
	))
}

// observeEsmfoldPlddt records an ESMFold mean pLDDT (panel 12, esmfold drain).
func observeEsmfoldPlddt(ctx context.Context, v float64) {
	if genomeEsmfoldPlddt == nil {
		return
	}
	genomeEsmfoldPlddt.Record(ctx, v)
}

// logMetricsInitFailure centralises the degraded-mode warning so the call site
// in main() stays terse.
func logMetricsInitFailure(err error) {
	log.Printf("WARNING: genome distribution metrics disabled (init failed: %v); drain continues without them", err)
}
