package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- Fake external-service clients for hermetic unit tests ---

type fakeUniProt struct {
	accByGene map[string]string
	seqByAcc  map[string]string
	geneErr   error
	seqErr    error
}

func (f *fakeUniProt) AccessionForGene(_ context.Context, gene string) (string, error) {
	if f.geneErr != nil {
		return "", f.geneErr
	}
	acc, ok := f.accByGene[gene]
	if !ok {
		return "", newResolveError(ECodeUniProtNotFound, "no acc for "+gene, nil)
	}
	return acc, nil
}

func (f *fakeUniProt) CanonicalSequence(_ context.Context, acc string) (string, error) {
	if f.seqErr != nil {
		return "", f.seqErr
	}
	seq, ok := f.seqByAcc[acc]
	if !ok {
		return "", newResolveError(ECodeUniProtNotFound, "no seq for "+acc, nil)
	}
	return seq, nil
}

type fakeEnsembl struct {
	gene    string
	residue int
	wt, mut byte
	err     error
}

func (f *fakeEnsembl) MissenseConsequence(_ context.Context, _ string) (string, int, byte, byte, error) {
	if f.err != nil {
		return "", 0, 0, 0, f.err
	}
	return f.gene, f.residue, f.wt, f.mut, nil
}

type fakeAlphaFold struct {
	pdb   []byte
	plddt *float64
	ok    bool
	err   error
}

func (f *fakeAlphaFold) FetchModel(_ context.Context, _ string) ([]byte, *float64, bool, error) {
	return f.pdb, f.plddt, f.ok, f.err
}

// A short canonical sequence where residue 5 is 'R' (index 4). TPMT-like stub.
// Positions: 1:M 2:D 3:G 4:T 5:R 6:T 7:S 8:L 9:D 10:I
const stubSeq = "MDGTRTSLDI"

func newFakeResolver(uni *fakeUniProt, ens *fakeEnsembl, af *fakeAlphaFold) *Resolver {
	fixed := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	return &Resolver{uniprot: uni, ensembl: ens, alphafold: af, now: func() time.Time { return fixed }}
}

// --- parseProteinChange table tests ---

func TestParseProteinChange(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		residue int
		wt, mut byte
		code    string // "" => success
	}{
		{"three-letter p-prefixed", "p.Arg117His", 117, 'R', 'H', ""},
		{"three-letter bare", "Arg117His", 117, 'R', 'H', ""},
		{"one-letter p-prefixed", "p.R117H", 117, 'R', 'H', ""},
		{"one-letter bare", "R117H", 117, 'R', 'H', ""},
		{"lowercase three-letter", "p.arg117his", 117, 'R', 'H', ""},
		{"stop gain is not missense", "p.Arg117Ter", 0, 0, 0, ECodeVariantNotMissense},
		{"stop one-letter", "R117*", 0, 0, 0, ECodeVariantNotMissense},
		{"synonymous rejected", "R117R", 0, 0, 0, ECodeVariantNotMissense},
		{"frameshift rejected", "p.Arg117fs", 0, 0, 0, ECodeVariantNotMissense},
		{"deletion rejected", "p.Arg117del", 0, 0, 0, ECodeVariantNotMissense},
		{"empty rejected", "", 0, 0, 0, ECodeParamsInvalid},
		{"garbage rejected", "hello", 0, 0, 0, ECodeParamsInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, wt, mut, err := parseProteinChange(c.in)
			if c.code == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if r != c.residue || wt != c.wt || mut != c.mut {
					t.Fatalf("got (%d,%c,%c), want (%d,%c,%c)", r, wt, mut, c.residue, c.wt, c.mut)
				}
				return
			}
			code, ok := ResolveErrorCode(err)
			if !ok || code != c.code {
				t.Fatalf("got error %v (code %q), want code %q", err, code, c.code)
			}
		})
	}
}

// --- canonical-key determinism: same substitution, different input modes ---

func TestResolutionKeyDeterminism(t *testing.T) {
	keyA, idA := resolutionKey("P51580", "P51580-1", 117, 'R', 'H')
	keyB, idB := resolutionKey("P51580", "P51580-1", 117, 'R', 'H')
	if idA != idB {
		t.Fatalf("non-deterministic key: %q vs %q", idA, idB)
	}
	if !strings.HasPrefix(idA, "rv-") || len(idA) != len("rv-")+16 {
		t.Fatalf("unexpected resolution_id shape: %q", idA)
	}
	// H1: the canonical key is source-INDEPENDENT. The same substitution must yield
	// ONE resolution_id regardless of where the structure came from, so an
	// AlphaFold hit and a later esmfold fallback converge on a single row. The key
	// string therefore must not encode any structure source.
	if strings.Contains(keyA, StructureSourceAlphaFold) ||
		strings.Contains(keyA, StructureSourceESMFold) ||
		strings.Contains(keyA, StructureSourcePDB) {
		t.Fatalf("canonical key must not include structure_source: %q", keyA)
	}
	if keyA != keyB {
		t.Fatalf("non-deterministic key string: %q vs %q", keyA, keyB)
	}
	if want := "P51580:P51580-1:117:R>H"; keyA != want {
		t.Fatalf("unexpected canonical key: got %q want %q", keyA, want)
	}
	// A different substitution must still produce a different id.
	_, idOther := resolutionKey("P51580", "P51580-1", 200, 'A', 'T')
	if idOther == idA {
		t.Fatalf("distinct substitutions collided on id %q", idA)
	}
}

// --- WT validation ---

func TestValidateWildType(t *testing.T) {
	// residue 5 of stubSeq is 'R'.
	if err := validateWildType(stubSeq, 5, 'R'); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	// mismatch -> E_WT_MISMATCH
	err := validateWildType(stubSeq, 5, 'K')
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeWTMismatch {
		t.Fatalf("expected E_WT_MISMATCH, got %v", err)
	}
	// out of range -> E_WT_MISMATCH
	err = validateWildType(stubSeq, 999, 'R')
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeWTMismatch {
		t.Fatalf("expected E_WT_MISMATCH for out-of-range, got %v", err)
	}
}

// --- normalize across modes (no DB, no network beyond fakes) ---

func TestNormalizeRSIDMissense(t *testing.T) {
	rsv := newFakeResolver(
		&fakeUniProt{},
		&fakeEnsembl{gene: "TPMT", residue: 5, wt: 'R', mut: 'H'},
		&fakeAlphaFold{},
	)
	norm, err := rsv.normalize(context.Background(), VariantInput{Mode: VariantModeRSID, RSID: "rs1142345"})
	if err != nil {
		t.Fatalf("normalize rsid: %v", err)
	}
	if norm.gene != "TPMT" || norm.residue != 5 || norm.wtAA != 'R' || norm.mutAA != 'H' {
		t.Fatalf("unexpected normalized: %+v", norm)
	}
}

func TestNormalizeRSIDNonMissense(t *testing.T) {
	rsv := newFakeResolver(
		&fakeUniProt{},
		&fakeEnsembl{err: newResolveError(ECodeVariantNotMissense, "non-missense", nil)},
		&fakeAlphaFold{},
	)
	_, err := rsv.normalize(context.Background(), VariantInput{Mode: VariantModeRSID, RSID: "rs999"})
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeVariantNotMissense {
		t.Fatalf("expected E_VARIANT_NOT_MISSENSE, got %v", err)
	}
}

func TestNormalizeHGVSProtein(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{}, &fakeAlphaFold{})
	norm, err := rsv.normalize(context.Background(),
		VariantInput{Mode: VariantModeHGVS, HGVS: "P51580:p.Arg117His"})
	if err != nil {
		t.Fatalf("normalize hgvs: %v", err)
	}
	if norm.uniprotAcc != "P51580" || norm.residue != 117 || norm.wtAA != 'R' || norm.mutAA != 'H' {
		t.Fatalf("unexpected normalized: %+v", norm)
	}
}

func TestNormalizeHGVSCodingRejected(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{}, &fakeAlphaFold{})
	_, err := rsv.normalize(context.Background(),
		VariantInput{Mode: VariantModeHGVS, HGVS: "ENST00000309983.6:c.349C>T"})
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
		t.Fatalf("expected E_PARAMS_INVALID for coding HGVS, got %v", err)
	}
}

// --- selectMissense unit (multi-gene + first-missense selection) ---

func TestSelectMissense(t *testing.T) {
	cons := []vepConsequence{
		{GeneSymbol: "TPMT", ConsequenceTerms: []string{"missense_variant"}, ProteinStart: 117, AminoAcids: "R/H"},
		{GeneSymbol: "TPMT", ConsequenceTerms: []string{"synonymous_variant"}, ProteinStart: 50, AminoAcids: "L/L"},
	}
	gene, res, wt, mut, genes, found := selectMissense(cons)
	if !found || gene != "TPMT" || res != 117 || wt != 'R' || mut != 'H' || len(genes) != 1 {
		t.Fatalf("unexpected: %s %d %c %c %v %v", gene, res, wt, mut, genes, found)
	}

	multi := []vepConsequence{
		{GeneSymbol: "GENEA", ConsequenceTerms: []string{"missense_variant"}, ProteinStart: 10, AminoAcids: "A/T"},
		{GeneSymbol: "GENEB", ConsequenceTerms: []string{"missense_variant"}, ProteinStart: 20, AminoAcids: "G/S"},
	}
	_, _, _, _, genes2, _ := selectMissense(multi)
	if len(genes2) != 2 {
		t.Fatalf("expected 2 genes for multi-gene rsID, got %v", genes2)
	}
}

// --- resolveStructure: AlphaFold hit vs miss (no DB) ---

func TestResolveStructureAlphaFoldHit(t *testing.T) {
	plddt := 92.4
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{},
		&fakeAlphaFold{pdb: []byte("ATOM CA stub"), plddt: &plddt, ok: true})
	_, rid := resolutionKey("P51580", "P51580-1", 117, 'R', 'H')
	rv := &ResolvedVariant{ResolutionID: rid, UniProtAcc: "P51580", UniProtIso: "P51580-1",
		ResidueIndex: 117, WildTypeAA: "R", MutantAA: "H"}
	if err := rsv.resolveStructure(context.Background(), &NoopS3Client{}, rv); err != nil {
		t.Fatalf("resolveStructure: %v", err)
	}
	if rv.StructureS3.Key != structureKey(rid) {
		t.Fatalf("AF-hit structure key %q must derive from the source-independent resolution_id", rv.StructureS3.Key)
	}
	if rv.StructureSrc != StructureSourceAlphaFold {
		t.Fatalf("expected alphafold source, got %q", rv.StructureSrc)
	}
	if rv.NeedsStructureFold {
		t.Fatalf("alphafold hit should not need fold")
	}
	if rv.StructureS3.Bucket != BucketStructures || !strings.HasSuffix(rv.StructureS3.Key, "structure.pdb") {
		t.Fatalf("unexpected S3 ref: %+v", rv.StructureS3)
	}
	if rv.StructureS3.PlddtGlobal == nil || *rv.StructureS3.PlddtGlobal != 92.4 {
		t.Fatalf("expected plddt 92.4, got %v", rv.StructureS3.PlddtGlobal)
	}
}

func TestResolveStructureESMFoldFallback(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{}, &fakeAlphaFold{ok: false})
	_, rid := resolutionKey("Q9NEW1", "Q9NEW1-1", 42, 'A', 'T')
	rv := &ResolvedVariant{ResolutionID: rid, UniProtAcc: "Q9NEW1", UniProtIso: "Q9NEW1-1",
		ResidueIndex: 42, WildTypeAA: "A", MutantAA: "T"}
	if err := rsv.resolveStructure(context.Background(), &NoopS3Client{}, rv); err != nil {
		t.Fatalf("resolveStructure: %v", err)
	}
	// H1 convergence: the esmfold fallback keys its (intended) structure path on the
	// SAME source-independent resolution_id an AlphaFold hit would have used, so a
	// later AlphaFold model lands on exactly the same S3 path / row.
	if rv.StructureS3.Key != structureKey(rid) {
		t.Fatalf("esmfold structure key %q must derive from the source-independent resolution_id", rv.StructureS3.Key)
	}
	if rv.StructureSrc != StructureSourceESMFold {
		t.Fatalf("expected esmfold source on AF miss, got %q", rv.StructureSrc)
	}
	if !rv.NeedsStructureFold {
		t.Fatalf("AF miss must set NeedsStructureFold for GEN-12 handoff")
	}
	if rv.StructureS3.PlddtGlobal != nil {
		t.Fatalf("plddt must be nil until fold completes")
	}
}

// H1: an AlphaFold hit and an esmfold fallback for the SAME substitution must
// converge on ONE source-independent resolution_id and ONE structure S3 path. This
// is what lets a later esmfold fold (or a later AlphaFold model) update the SAME
// variant_resolutions row in place instead of minting a competing resolution.
func TestAlphaFoldAndESMFoldConvergeOnOneResolutionID(t *testing.T) {
	// Same substitution coordinates for both resolutions.
	acc, iso, residue, wt, mut := "P51580", "P51580-1", 117, byte('R'), byte('H')
	_, wantID := resolutionKey(acc, iso, residue, wt, mut)

	// AlphaFold-hit resolution.
	plddt := 91.0
	afResolver := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{},
		&fakeAlphaFold{pdb: []byte("ATOM CA stub"), plddt: &plddt, ok: true})
	_, afID := resolutionKey(acc, iso, residue, wt, mut)
	afRV := &ResolvedVariant{ResolutionID: afID, UniProtAcc: acc, UniProtIso: iso,
		ResidueIndex: residue, WildTypeAA: string(wt), MutantAA: string(mut)}
	if err := afResolver.resolveStructure(context.Background(), &NoopS3Client{}, afRV); err != nil {
		t.Fatalf("alphafold resolveStructure: %v", err)
	}

	// esmfold-fallback resolution of the SAME substitution (AlphaFold miss).
	esmResolver := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{}, &fakeAlphaFold{ok: false})
	_, esmID := resolutionKey(acc, iso, residue, wt, mut)
	esmRV := &ResolvedVariant{ResolutionID: esmID, UniProtAcc: acc, UniProtIso: iso,
		ResidueIndex: residue, WildTypeAA: string(wt), MutantAA: string(mut)}
	if err := esmResolver.resolveStructure(context.Background(), &NoopS3Client{}, esmRV); err != nil {
		t.Fatalf("esmfold resolveStructure: %v", err)
	}

	if afRV.ResolutionID != wantID || esmRV.ResolutionID != wantID {
		t.Fatalf("resolution_ids diverge: alphafold=%q esmfold=%q want=%q",
			afRV.ResolutionID, esmRV.ResolutionID, wantID)
	}
	if afRV.ResolutionID != esmRV.ResolutionID {
		t.Fatalf("alphafold and esmfold of the same substitution must share one resolution_id, got %q vs %q",
			afRV.ResolutionID, esmRV.ResolutionID)
	}
	if afRV.StructureS3.Key != esmRV.StructureS3.Key {
		t.Fatalf("both sources must key the structure on the same S3 path, got %q vs %q",
			afRV.StructureS3.Key, esmRV.StructureS3.Key)
	}
	// Sources differ (mutable column), identity does not.
	if afRV.StructureSrc != StructureSourceAlphaFold || esmRV.StructureSrc != StructureSourceESMFold {
		t.Fatalf("expected alphafold/esmfold sources, got %q/%q", afRV.StructureSrc, esmRV.StructureSrc)
	}
}

func TestResolveStructureUpstreamError(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{},
		&fakeAlphaFold{err: newResolveError(ECodeResolveUpstream, "ebi down", nil)})
	rv := &ResolvedVariant{UniProtAcc: "P00000", UniProtIso: "P00000-1",
		ResidueIndex: 1, WildTypeAA: "M", MutantAA: "V"}
	err := rsv.resolveStructure(context.Background(), &NoopS3Client{}, rv)
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeResolveUpstream {
		t.Fatalf("expected E_RESOLVE_UPSTREAM, got %v", err)
	}
}

// --- meanCABFactor (global pLDDT extraction) ---

func TestMeanCABFactor(t *testing.T) {
	// Two CA atoms with B-factors 90.00 and 80.00 -> mean 85.00.
	pdb := "ATOM      1  CA  MET A   1      11.104  13.207  10.567  1.00 90.00           C\n" +
		"ATOM      2  CA  ASP A   2      12.104  14.207  11.567  1.00 80.00           C\n"
	got := meanCABFactor([]byte(pdb))
	if got == nil {
		t.Fatal("expected a pLDDT mean, got nil")
	}
	if *got < 84.9 || *got > 85.1 {
		t.Fatalf("expected ~85.0, got %v", *got)
	}
	if meanCABFactor([]byte("HEADER only, no atoms")) != nil {
		t.Fatal("expected nil for a PDB with no CA atoms")
	}
}

// --- VULN-4: identifier validation at the adapter boundary ---

func TestValidateUniProtAcc(t *testing.T) {
	// O/P/Q-led accessions (O60260, P51580, Q9Y6K9) are valid per the full UniProtKB
	// grammar -- the naive [A-NR-Z] first-class wrongly excluded them. They MUST pass.
	valid := []string{"P51580", "Q9Y6K9", "O60260", "A0A024R161", "P51580-2", "A0A024R161-10"}
	for _, acc := range valid {
		if err := validateUniProtAcc(acc); err != nil {
			t.Errorf("expected %q to be valid, got %v", acc, err)
		}
	}
	// Empty is allowed (caller decides whether an accession is required).
	if err := validateUniProtAcc(""); err != nil {
		t.Errorf("empty acc should be allowed, got %v", err)
	}
	invalid := []string{
		"P515",          // too short
		"P51580XXXXXXX", // too long (not a 6- or 10-char form)
		"O123456",       // 7 chars: not a valid 6- or 10-char accession form
		"P51580/../etc", // path traversal attempt
		"P51580?x=1",    // query injection attempt
		"P51580 OR 1=1", // space / injection
		"p51580",        // lowercase rejected
		"P51580-",       // dangling isoform separator
	}
	for _, acc := range invalid {
		err := validateUniProtAcc(acc)
		if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
			t.Errorf("expected E_PARAMS_INVALID for %q, got %v", acc, err)
		}
	}
}

func TestValidateGene(t *testing.T) {
	for _, g := range []string{"TPMT", "HLA-DRB1", "C4orf3", "RP11-1.2", "BRCA1"} {
		if err := validateGene(g); err != nil {
			t.Errorf("expected gene %q valid, got %v", g, err)
		}
	}
	if err := validateGene(""); err != nil {
		t.Errorf("empty gene should be allowed, got %v", err)
	}
	for _, g := range []string{"TPMT OR 1=1", "TP MT", "gene&q=x", "g/../e", "TPMT\n"} {
		err := validateGene(g)
		if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
			t.Errorf("expected E_PARAMS_INVALID for gene %q, got %v", g, err)
		}
	}
}

func TestValidateRSID(t *testing.T) {
	for _, r := range []string{"rs1142345", "rs1", "rs999999999"} {
		if err := validateRSID(r); err != nil {
			t.Errorf("expected rsid %q valid, got %v", r, err)
		}
	}
	for _, r := range []string{"", "rs", "rsABC", "RS123", "rs123 OR 1=1", "rs123/../x", "1142345"} {
		err := validateRSID(r)
		if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
			t.Errorf("expected E_PARAMS_INVALID for rsid %q, got %v", r, err)
		}
	}
}

// resolveUniProt must reject a malformed caller-supplied accession before any
// network use (the operator-override SSRF surface).
func TestResolveUniProtRejectsBadAcc(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{seqByAcc: map[string]string{"P51580": stubSeq}}, &fakeEnsembl{}, &fakeAlphaFold{})
	in := VariantInput{Mode: VariantModeProteinChange, UniProtAcc: "P51580/../evil"}
	norm := normalized{uniprotAcc: "P51580/../evil", residue: 5, wtAA: 'R', mutAA: 'T'}
	_, _, _, _, err := rsv.resolveUniProt(context.Background(), in, norm)
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
		t.Fatalf("expected E_PARAMS_INVALID for crafted acc, got %v", err)
	}
}

// rsid mode must reject a malformed rsID before the Ensembl request.
func TestNormalizeRSIDRejectsBadRSID(t *testing.T) {
	rsv := newFakeResolver(&fakeUniProt{}, &fakeEnsembl{gene: "TPMT", residue: 5, wt: 'R', mut: 'H'}, &fakeAlphaFold{})
	_, err := rsv.normalize(context.Background(), VariantInput{Mode: VariantModeRSID, RSID: "rs123/../x"})
	if code, ok := ResolveErrorCode(err); !ok || code != ECodeParamsInvalid {
		t.Fatalf("expected E_PARAMS_INVALID for crafted rsid, got %v", err)
	}
}
