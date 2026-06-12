// genome_resolve_uniprot.go: UniProt accession + canonical-sequence resolution
// and rsID -> missense-consequence resolution for the variant adapter
// (core TDD §5.2 step 1-2).
//
// Resolution-source decisions (documented per the GEN-10 brief, which left the
// rsID source open):
//   - UniProt accession + canonical sequence: UniProt REST. Gene-name search via
//     the REST stream/search API (rest.uniprot.org). When a UniProt accession is
//     pinned (operator override or HGVS prefix) we fetch its sequence directly.
//   - rsID -> protein consequence: Ensembl REST VEP (rest.ensembl.org), which is
//     the canonical source for the same VEP-derived protein_pos/ref_aa/alt_aa
//     u4u already computes. We take the first missense transcript consequence on
//     a single gene; non-missense / multi-gene -> E_VARIANT_NOT_MISSENSE.
//
// These endpoints CANNOT be exercised from the build host; the request/response
// shapes below follow the documented public API contracts and are noted as
// unverified-against-live in the GEN-10 report. All external calls carry a
// per-call timeout and translate transport/4xx/5xx into typed E_* errors.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- Client interfaces (mockable for unit tests) ---

// uniProtResolver resolves a gene symbol to a canonical UniProt accession and
// fetches a canonical protein sequence by accession.
type uniProtResolver interface {
	// AccessionForGene returns the canonical (reviewed/Swiss-Prot, human)
	// UniProt accession for an HGNC gene symbol.
	AccessionForGene(ctx context.Context, gene string) (string, error)
	// CanonicalSequence returns the canonical protein sequence for an accession.
	CanonicalSequence(ctx context.Context, acc string) (string, error)
}

// ensemblResolver resolves an rsID to a missense protein consequence.
type ensemblResolver interface {
	// MissenseConsequence returns (gene, residueIndex, wtAA, mutAA) for the
	// single missense consequence of an rsID, or a typed error.
	MissenseConsequence(ctx context.Context, rsid string) (gene string, residue int, wt, mut byte, err error)
}

// alphaFoldClient fetches a precomputed AlphaFold DB model PDB by accession.
type alphaFoldClient interface {
	// FetchModel returns the PDB bytes for an accession, ok=false on a clean
	// "no model" miss (404), or a typed error on transport failure.
	FetchModel(ctx context.Context, acc string) (pdb []byte, plddtGlobal *float64, ok bool, err error)
}

// --- resolveUniProt: stage 2 of Resolve ---

// resolveUniProt fills in the UniProt accession (if not already known from the
// parse stage), the explicit isoform (defaulting to canonical "<acc>-1"), and
// the canonical sequence used for WT validation. It returns any informational
// warnings (e.g. transcript -> canonical isoform note).
func (rsv *Resolver) resolveUniProt(ctx context.Context, in VariantInput, norm normalized) (
	acc, isoform, sequence string, warnings []string, err error,
) {
	acc = norm.uniprotAcc
	if acc == "" {
		acc = in.UniProtAcc
	}
	// VULN-4: a caller-supplied accession (operator override or HGVS prefix) flows
	// straight into outbound UniProt/AlphaFold URLs. Validate it at this boundary
	// before any network use; reject a malformed accession with E_PARAMS_INVALID.
	if err = validateUniProtAcc(acc); err != nil {
		return "", "", "", nil, err
	}
	if acc == "" {
		// Need a gene to look up an accession.
		if norm.gene == "" {
			return "", "", "", nil, newResolveError(ECodeUniProtNotFound,
				"no uniprot_acc and no gene to resolve an accession from", nil)
		}
		// VULN-4: the gene symbol shapes the outbound UniProt search query.
		if err = validateGene(norm.gene); err != nil {
			return "", "", "", nil, err
		}
		acc, err = rsv.uniprot.AccessionForGene(ctx, norm.gene)
		if err != nil {
			return "", "", "", nil, err
		}
		// Defense in depth: even a resolver-returned accession is interpolated into
		// downstream URLs, so re-validate before it shapes the sequence/AlphaFold
		// fetches.
		if err = validateUniProtAcc(acc); err != nil {
			return "", "", "", nil, err
		}
	}

	// A caller may pass an isoform-suffixed accession (e.g. "P51580-2"); strip to
	// the bare accession before deriving the canonical isoform and fetching the
	// sequence — otherwise we'd build "P51580-2-1" and query a non-existent
	// isoform / break the upstream lookups. (Copilot review)
	if i := strings.IndexByte(acc, '-'); i > 0 {
		acc = acc[:i]
	}
	isoform = acc + "-1" // canonical isoform by default (core §5.2)

	sequence, err = rsv.uniprot.CanonicalSequence(ctx, acc)
	if err != nil {
		return "", "", "", nil, err
	}
	if sequence == "" {
		return "", "", "", nil, newResolveError(ECodeUniProtNotFound,
			fmt.Sprintf("UniProt %s returned an empty canonical sequence", acc), nil)
	}

	if norm.transcript != "" {
		warnings = append(warnings,
			fmt.Sprintf("transcript %s mapped to canonical isoform %s", norm.transcript, isoform))
	}
	return acc, isoform, sequence, warnings, nil
}

// normalizeRSID resolves an rsID to its missense protein consequence via the
// Ensembl client, then carries the gene forward so resolveUniProt can map it to
// a UniProt accession. Non-missense / multi-gene rsIDs are rejected upstream by
// the Ensembl client with E_VARIANT_NOT_MISSENSE.
func (rsv *Resolver) normalizeRSID(ctx context.Context, in VariantInput) (normalized, error) {
	rsid := strings.TrimSpace(in.RSID)
	if rsid == "" {
		return normalized{}, newResolveError(ECodeParamsInvalid, "rsid mode requires rsid", nil)
	}
	// VULN-4: the rsID shapes the outbound Ensembl VEP request path. Enforce the
	// strict "rs<digits>" grammar (supersedes the old loose rs-prefix check) before
	// it is used; anything else is rejected with E_PARAMS_INVALID.
	if err := validateRSID(rsid); err != nil {
		return normalized{}, err
	}
	gene, residue, wt, mut, err := rsv.ensembl.MissenseConsequence(ctx, rsid)
	if err != nil {
		return normalized{}, err
	}
	return normalized{
		uniprotAcc: in.UniProtAcc, // honor an explicit override if present
		residue:    residue,
		wtAA:       wt,
		mutAA:      mut,
		gene:       gene,
		transcript: in.Transcript,
	}, nil
}

// --- HTTP-backed implementations ---

const (
	uniProtBaseURL   = "https://rest.uniprot.org"
	ensemblBaseURL   = "https://rest.ensembl.org"
	alphaFoldFileFmt = "https://alphafold.ebi.ac.uk/files/AF-%s-F1-model_v4.pdb"
)

// httpUniProt implements uniProtResolver against rest.uniprot.org.
type httpUniProt struct {
	http    *http.Client
	baseURL string
}

func newHTTPUniProtResolver() *httpUniProt {
	return &httpUniProt{http: &http.Client{Timeout: 30 * time.Second}, baseURL: uniProtBaseURL}
}

// AccessionForGene queries the UniProtKB search API for a reviewed human entry
// matching the gene symbol and returns its primary accession.
//
// GET /uniprotkb/search?query=gene_exact:{gene}+AND+organism_id:9606+AND+reviewed:true
//
//	&fields=accession&format=json&size=1
func (u *httpUniProt) AccessionForGene(ctx context.Context, gene string) (string, error) {
	q := url.Values{}
	q.Set("query", fmt.Sprintf("gene_exact:%s AND organism_id:9606 AND reviewed:true", gene))
	q.Set("fields", "accession")
	q.Set("format", "json")
	q.Set("size", "1")
	endpoint := u.baseURL + "/uniprotkb/search?" + q.Encode()

	body, status, err := httpGetJSON(ctx, u.http, endpoint)
	if err != nil {
		return "", newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("UniProt gene search for %q failed", gene), err)
	}
	if status == http.StatusNotFound {
		return "", newResolveError(ECodeUniProtNotFound, fmt.Sprintf("UniProt gene %q not found", gene), nil)
	}
	if status >= 500 {
		return "", newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("UniProt gene search returned %d for %q", status, gene), nil)
	}
	if status != http.StatusOK {
		return "", newResolveError(ECodeUniProtNotFound,
			fmt.Sprintf("UniProt gene search returned %d for %q", status, gene), nil)
	}

	var parsed struct {
		Results []struct {
			PrimaryAccession string `json:"primaryAccession"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", newResolveError(ECodeResolveUpstream, "decoding UniProt gene search response", err)
	}
	if len(parsed.Results) == 0 || parsed.Results[0].PrimaryAccession == "" {
		return "", newResolveError(ECodeUniProtNotFound,
			fmt.Sprintf("no reviewed human UniProt entry for gene %q", gene), nil)
	}
	return parsed.Results[0].PrimaryAccession, nil
}

// CanonicalSequence fetches the canonical protein sequence for an accession.
//
// GET /uniprotkb/{acc}?fields=sequence&format=json
func (u *httpUniProt) CanonicalSequence(ctx context.Context, acc string) (string, error) {
	endpoint := u.baseURL + "/uniprotkb/" + url.PathEscape(acc) + "?fields=sequence&format=json"

	body, status, err := httpGetJSON(ctx, u.http, endpoint)
	if err != nil {
		return "", newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("UniProt sequence fetch for %q failed", acc), err)
	}
	if status == http.StatusNotFound {
		return "", newResolveError(ECodeUniProtNotFound, fmt.Sprintf("UniProt accession %q not found", acc), nil)
	}
	if status >= 500 {
		return "", newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("UniProt sequence fetch returned %d for %q", status, acc), nil)
	}
	if status != http.StatusOK {
		return "", newResolveError(ECodeUniProtNotFound,
			fmt.Sprintf("UniProt sequence fetch returned %d for %q", status, acc), nil)
	}

	var parsed struct {
		Sequence struct {
			Value string `json:"value"`
		} `json:"sequence"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", newResolveError(ECodeResolveUpstream, "decoding UniProt sequence response", err)
	}
	return strings.TrimSpace(parsed.Sequence.Value), nil
}

// httpEnsembl implements ensemblResolver against rest.ensembl.org VEP.
type httpEnsembl struct {
	http    *http.Client
	baseURL string
}

func newHTTPEnsemblResolver() *httpEnsembl {
	return &httpEnsembl{http: &http.Client{Timeout: 30 * time.Second}, baseURL: ensemblBaseURL}
}

// MissenseConsequence resolves an rsID to a single missense protein consequence
// via Ensembl VEP.
//
// GET /vep/human/id/{rsid}?content-type=application/json
//
// The response is an array of variant objects, each carrying
// transcript_consequences[] with consequence_terms, gene_symbol, protein_start,
// amino_acids ("R/H"). We select the first transcript consequence whose terms
// include "missense_variant". If consequences span multiple gene symbols, or
// none is missense, we reject with E_VARIANT_NOT_MISSENSE (core R6).
func (e *httpEnsembl) MissenseConsequence(ctx context.Context, rsid string) (string, int, byte, byte, error) {
	endpoint := e.baseURL + "/vep/human/id/" + url.PathEscape(rsid) + "?content-type=application/json"

	body, status, err := httpGetJSON(ctx, e.http, endpoint)
	if err != nil {
		return "", 0, 0, 0, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("Ensembl VEP lookup for %q failed", rsid), err)
	}
	if status == http.StatusNotFound {
		return "", 0, 0, 0, newResolveError(ECodeVariantNotMissense,
			fmt.Sprintf("Ensembl VEP has no record for %q", rsid), nil)
	}
	if status >= 500 {
		return "", 0, 0, 0, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("Ensembl VEP returned %d for %q", status, rsid), nil)
	}
	if status != http.StatusOK {
		return "", 0, 0, 0, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("Ensembl VEP returned %d for %q", status, rsid), nil)
	}

	var variants []struct {
		TranscriptConsequences []vepConsequence `json:"transcript_consequences"`
	}
	if err := json.Unmarshal(body, &variants); err != nil {
		return "", 0, 0, 0, newResolveError(ECodeResolveUpstream, "decoding Ensembl VEP response", err)
	}

	var all []vepConsequence
	for _, v := range variants {
		all = append(all, v.TranscriptConsequences...)
	}

	gene, residue, wt, mut, genes, found := selectMissense(all)
	if !found {
		return "", 0, 0, 0, newResolveError(ECodeVariantNotMissense,
			fmt.Sprintf("rsID %q has no missense protein consequence", rsid), nil)
	}
	if len(genes) > 1 {
		return "", 0, 0, 0, newResolveError(ECodeVariantNotMissense,
			fmt.Sprintf("rsID %q maps to multiple genes %v; missense-scoped calcs require a single gene",
				rsid, genes), nil)
	}
	return gene, residue, wt, mut, nil
}

// vepConsequence is the minimal shape of a VEP transcript_consequences entry.
type vepConsequence struct {
	GeneSymbol       string   `json:"gene_symbol"`
	ConsequenceTerms []string `json:"consequence_terms"`
	ProteinStart     int      `json:"protein_start"`
	AminoAcids       string   `json:"amino_acids"` // "R/H"
}

// selectMissense scans VEP transcript consequences for the first missense
// substitution and collects the distinct gene symbols across ALL missense
// consequences (for the multi-gene guard). Split out for unit testing without HTTP.
func selectMissense(consequences []vepConsequence) (gene string, residue int, wt, mut byte, genes []string, found bool) {
	geneSet := map[string]bool{}
	for _, tc := range consequences {
		if !termsContain(tc.ConsequenceTerms, "missense_variant") {
			continue
		}
		r, w, m, ok := parseAminoAcids(tc.AminoAcids, tc.ProteinStart)
		if !ok {
			continue
		}
		if tc.GeneSymbol != "" {
			geneSet[tc.GeneSymbol] = true
		}
		if !found {
			gene, residue, wt, mut, found = tc.GeneSymbol, r, w, m, true
		}
	}
	for g := range geneSet {
		genes = append(genes, g)
	}
	return gene, residue, wt, mut, genes, found
}

// parseAminoAcids parses a VEP "R/H" amino_acids field + protein_start into
// (residue, wt, mut). Returns ok=false for non-single-substitution shapes.
func parseAminoAcids(aa string, proteinStart int) (int, byte, byte, bool) {
	parts := strings.Split(aa, "/")
	if len(parts) != 2 || len(parts[0]) != 1 || len(parts[1]) != 1 {
		return 0, 0, 0, false
	}
	wt := upperByte(parts[0][0])
	mut := upperByte(parts[1][0])
	if !oneLetterAA[wt] || !oneLetterAA[mut] || wt == mut || proteinStart < 1 {
		return 0, 0, 0, false
	}
	return proteinStart, wt, mut, true
}

func termsContain(terms []string, want string) bool {
	for _, t := range terms {
		if t == want {
			return true
		}
	}
	return false
}

// httpAlphaFold implements alphaFoldClient against the AlphaFold DB file server.
type httpAlphaFold struct {
	http   *http.Client
	urlFmt string
}

func newHTTPAlphaFoldClient() *httpAlphaFold {
	return &httpAlphaFold{http: &http.Client{Timeout: 60 * time.Second}, urlFmt: alphaFoldFileFmt}
}

// FetchModel downloads AF-{acc}-F1-model_v4.pdb. A 404 is a clean "no model"
// miss (ok=false, nil error) that triggers the ESMFold fallback. Transport and
// 5xx failures return a typed E_RESOLVE_UPSTREAM error. The global pLDDT is
// parsed from the PDB B-factor column mean over CA atoms (AlphaFold encodes
// per-residue pLDDT in the B-factor).
func (a *httpAlphaFold) FetchModel(ctx context.Context, acc string) ([]byte, *float64, bool, error) {
	// VULN-4 defense in depth: the accession is already validated at the adapter
	// boundary (validateUniProtAcc), but PathEscape it before interpolation so a
	// stray separator can never break out of the AlphaFold file path segment.
	endpoint := fmt.Sprintf(a.urlFmt, url.PathEscape(acc))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, false, newResolveError(ECodeResolveUpstream, "building AlphaFold request", err)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, nil, false, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("AlphaFold fetch for %q failed", acc), err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil, false, nil // clean miss -> ESMFold fallback
	case resp.StatusCode >= 500:
		return nil, nil, false, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("AlphaFold returned %d for %q", resp.StatusCode, acc), nil)
	case resp.StatusCode != http.StatusOK:
		// Treat other non-200 as a miss rather than a hard failure (the model
		// simply is not available); the fallback path is the safe default.
		return nil, nil, false, nil
	}

	pdb, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MiB cap
	if err != nil {
		return nil, nil, false, newResolveError(ECodeResolveUpstream, "reading AlphaFold PDB body", err)
	}
	plddt := meanCABFactor(pdb)
	return pdb, plddt, true, nil
}

// meanCABFactor computes the mean B-factor over CA atoms of a PDB, which is the
// global pLDDT for AlphaFold models. Returns nil if no CA atoms are found.
func meanCABFactor(pdb []byte) *float64 {
	var sum float64
	var n int
	for _, line := range strings.Split(string(pdb), "\n") {
		if !strings.HasPrefix(line, "ATOM") || len(line) < 66 {
			continue
		}
		// Columns 13-16 atom name; 61-66 tempFactor (PDB fixed-width, 1-based).
		atomName := strings.TrimSpace(line[12:16])
		if atomName != "CA" {
			continue
		}
		var b float64
		if _, err := fmt.Sscanf(strings.TrimSpace(line[60:66]), "%f", &b); err == nil {
			sum += b
			n++
		}
	}
	if n == 0 {
		return nil
	}
	mean := sum / float64(n)
	return &mean
}

// httpGetJSON performs a GET and returns the body bytes + status code. The
// caller maps status to typed errors. Transport errors are returned as err.
func httpGetJSON(ctx context.Context, client *http.Client, endpoint string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
