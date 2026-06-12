// genome_resolve_parse.go: mode-specific input parsing for the variant adapter
// (core TDD §5.2 step 1). Each of the three submission modes is normalized to
// the minimal substitution coordinates (residue, wt_aa, mut_aa) plus whatever
// is known about the UniProt anchor (gene / transcript / explicit accession).
// UniProt accession + canonical-sequence resolution itself lives in
// genome_resolve_uniprot.go; this file is pure string parsing.
package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// aa3to1 maps three-letter amino-acid codes to one-letter codes, including the
// stop pseudo-residue "Ter"/"*". Used to parse p.Arg117His-style changes.
var aa3to1 = map[string]byte{
	"Ala": 'A', "Arg": 'R', "Asn": 'N', "Asp": 'D', "Cys": 'C',
	"Gln": 'Q', "Glu": 'E', "Gly": 'G', "His": 'H', "Ile": 'I',
	"Leu": 'L', "Lys": 'K', "Met": 'M', "Phe": 'F', "Pro": 'P',
	"Ser": 'S', "Thr": 'T', "Trp": 'W', "Tyr": 'Y', "Val": 'V',
	"Sec": 'U', "Pyl": 'O', "Ter": '*',
}

// oneLetterAA is the set of valid single-letter residues (incl. selenocysteine
// U and pyrrolysine O). "*" denotes a stop and is rejected for missense calcs.
var oneLetterAA = func() map[byte]bool {
	m := map[byte]bool{}
	for _, c := range "ACDEFGHIKLMNPQRSTVWYUO" {
		m[byte(c)] = true
	}
	return m
}()

// normalize dispatches on in.Mode and returns the parsed substitution
// coordinates. UniProt resolution (gene/rsID -> accession + sequence) happens
// in a later stage; for rsID mode the residue/wt/mut are filled by an Ensembl
// VEP lookup performed here (it is the only way to obtain a protein change from
// an rsID), so this method needs ctx and the resolver's ensembl client.
func (rsv *Resolver) normalize(ctx context.Context, in VariantInput) (normalized, error) {
	switch in.Mode {
	case VariantModeProteinChange:
		return rsv.normalizeProteinChange(in)
	case VariantModeHGVS:
		return rsv.normalizeHGVS(in)
	case VariantModeRSID:
		return rsv.normalizeRSID(ctx, in)
	default:
		return normalized{}, newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("unknown variant mode %q (expected protein_change|hgvs|rsid)", in.Mode), nil)
	}
}

// normalizeProteinChange parses gene + protein change. The protein change may be
// 3-letter ("p.Arg117His"), 1-letter ("p.R117H"), or bare ("R117H").
func (rsv *Resolver) normalizeProteinChange(in VariantInput) (normalized, error) {
	if in.Gene == "" && in.UniProtAcc == "" {
		return normalized{}, newResolveError(ECodeParamsInvalid,
			"protein_change mode requires gene or uniprot_acc", nil)
	}
	if in.ProteinChange == "" {
		return normalized{}, newResolveError(ECodeParamsInvalid,
			"protein_change mode requires protein_change", nil)
	}
	residue, wt, mut, err := parseProteinChange(in.ProteinChange)
	if err != nil {
		return normalized{}, err
	}
	return normalized{
		uniprotAcc: in.UniProtAcc,
		residue:    residue,
		wtAA:       wt,
		mutAA:      mut,
		gene:       in.Gene,
		transcript: in.Transcript,
	}, nil
}

// parseProteinChange parses a single-substitution protein change into
// (residue, wtAA, mutAA). Accepts:
//   - 3-letter: Arg117His (optionally p.-prefixed)
//   - 1-letter: R117H     (optionally p.-prefixed)
//
// It rejects ranges, frameshifts, deletions, insertions, and stop-gain/loss
// (anything that is not a clean single missense substitution) with
// E_VARIANT_NOT_MISSENSE, since the four calcs are missense-scoped by design.
func parseProteinChange(raw string) (int, byte, byte, error) {
	s := trimProteinPrefix(raw)
	if s == "" {
		return 0, 0, 0, newResolveError(ECodeParamsInvalid, "empty protein change", nil)
	}
	// Reject obvious non-substitution syntaxes early with the missense-scope code.
	for _, marker := range []string{"del", "ins", "dup", "fs", "_", "ext", "=", ">"} {
		if strings.Contains(strings.ToLower(s), marker) {
			return 0, 0, 0, newResolveError(ECodeVariantNotMissense,
				fmt.Sprintf("protein change %q is not a single missense substitution", raw), nil)
		}
	}

	// Try 3-letter form: <3 letters><digits><3 letters>.
	if r, wt, mut, ok := parse3Letter(s); ok {
		return finishParse(raw, r, wt, mut)
	}
	// Try 1-letter form: <letter><digits><letter>.
	if r, wt, mut, ok := parse1Letter(s); ok {
		return finishParse(raw, r, wt, mut)
	}
	return 0, 0, 0, newResolveError(ECodeParamsInvalid,
		fmt.Sprintf("unparseable protein change %q (expected e.g. p.Arg117His or R117H)", raw), nil)
}

// finishParse validates the parsed residue/wt/mut and rejects stop residues as
// non-missense.
func finishParse(raw string, residue int, wt, mut byte) (int, byte, byte, error) {
	if residue < 1 {
		return 0, 0, 0, newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("protein change %q has non-positive residue index", raw), nil)
	}
	if wt == '*' || mut == '*' {
		return 0, 0, 0, newResolveError(ECodeVariantNotMissense,
			fmt.Sprintf("protein change %q involves a stop codon (not missense)", raw), nil)
	}
	if !oneLetterAA[wt] || !oneLetterAA[mut] {
		return 0, 0, 0, newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("protein change %q has an unrecognized amino acid", raw), nil)
	}
	if wt == mut {
		return 0, 0, 0, newResolveError(ECodeVariantNotMissense,
			fmt.Sprintf("protein change %q is synonymous (wt == mut)", raw), nil)
	}
	return residue, wt, mut, nil
}

// parse3Letter parses "Arg117His" -> (117,'R','H',true). Returns ok=false if
// the string is not in the 3-letter shape.
func parse3Letter(s string) (int, byte, byte, bool) {
	if len(s) < 7 { // 3 + >=1 digit + 3
		return 0, 0, 0, false
	}
	wt3 := s[:3]
	wt, ok := aa3to1[canonicalAA3(wt3)]
	if !ok {
		return 0, 0, 0, false
	}
	rest := s[3:]
	// Trailing 3 letters are the mutant code.
	if len(rest) < 4 {
		return 0, 0, 0, false
	}
	mut3 := rest[len(rest)-3:]
	mut, ok := aa3to1[canonicalAA3(mut3)]
	if !ok {
		return 0, 0, 0, false
	}
	digits := rest[:len(rest)-3]
	residue, err := strconv.Atoi(digits)
	if err != nil {
		return 0, 0, 0, false
	}
	return residue, wt, mut, true
}

// parse1Letter parses "R117H" -> (117,'R','H',true).
func parse1Letter(s string) (int, byte, byte, bool) {
	if len(s) < 3 {
		return 0, 0, 0, false
	}
	wt := upperByte(s[0])
	mut := upperByte(s[len(s)-1])
	// Accept a stop '*' in either position so finishParse can classify it as a
	// non-missense stop-gain/loss (rather than an unparseable string).
	wtOK := isLetter(wt) || wt == '*'
	mutOK := isLetter(mut) || mut == '*'
	if !wtOK || !mutOK {
		return 0, 0, 0, false
	}
	digits := s[1 : len(s)-1]
	if digits == "" {
		return 0, 0, 0, false
	}
	residue, err := strconv.Atoi(digits)
	if err != nil {
		return 0, 0, 0, false
	}
	return residue, wt, mut, true
}

// canonicalAA3 title-cases a 3-letter code ("ARG"/"arg" -> "Arg") for map lookup.
func canonicalAA3(s string) string {
	if len(s) != 3 {
		return s
	}
	return string(upperByte(s[0])) + strings.ToLower(s[1:])
}

func upperByte(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

func isLetter(b byte) bool { return (b >= 'A' && b <= 'Z') }

// normalizeHGVS parses an HGVS protein string (p.). Coding HGVS (c.) requires a
// transcript->protein projection that the adapter cannot do offline; for coding
// HGVS we route through Ensembl VEP (same path as rsID) when a transcript and
// the ensembl client are available, otherwise reject with E_PARAMS_INVALID and
// a clear message so the caller can resubmit as protein_change.
func (rsv *Resolver) normalizeHGVS(in VariantInput) (normalized, error) {
	if in.HGVS == "" {
		return normalized{}, newResolveError(ECodeParamsInvalid, "hgvs mode requires hgvs", nil)
	}
	h := strings.TrimSpace(in.HGVS)

	// Split an optional "ACCESSION:variant" prefix (e.g. "P51580:p.Arg117His"
	// or "ENST00000309983.6:c.349C>T").
	accPart, varPart := splitHGVS(h)

	switch {
	case strings.Contains(varPart, "p."):
		residue, wt, mut, err := parseProteinChange(varPart)
		if err != nil {
			return normalized{}, err
		}
		acc := in.UniProtAcc
		if acc == "" && looksLikeUniProtAcc(accPart) {
			acc = accPart
		}
		gene := in.Gene
		return normalized{
			uniprotAcc: acc,
			residue:    residue,
			wtAA:       wt,
			mutAA:      mut,
			gene:       gene,
			transcript: in.Transcript,
		}, nil
	case strings.HasPrefix(varPart, "c.") || strings.HasPrefix(varPart, "g.") || strings.HasPrefix(varPart, "n."):
		return normalized{}, newResolveError(ECodeParamsInvalid,
			"coding/genomic HGVS requires transcript->protein projection; submit as protein_change or rsid instead "+
				"(coding-HGVS projection via Ensembl VEP is a follow-up beyond the adapter core)", nil)
	default:
		return normalized{}, newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("unrecognized HGVS string %q", in.HGVS), nil)
	}
}

// splitHGVS splits "ACC:variant" into (acc, variant). With no colon, acc="".
func splitHGVS(h string) (string, string) {
	if i := strings.Index(h, ":"); i >= 0 {
		return strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:])
	}
	return "", h
}

// looksLikeUniProtAcc is a cheap heuristic for a UniProt accession (e.g. P51580,
// Q9Y6K9, A0A024R161). It avoids treating an Ensembl transcript as an accession.
func looksLikeUniProtAcc(s string) bool {
	if len(s) < 6 || len(s) > 10 {
		return false
	}
	if strings.HasPrefix(strings.ToUpper(s), "ENST") || strings.HasPrefix(strings.ToUpper(s), "NM_") {
		return false
	}
	// First char is a letter, second is a digit -> classic UniProt shape.
	return isLetter(upperByte(s[0])) && s[1] >= '0' && s[1] <= '9'
}

// --- VULN-4: input validation at the adapter boundary ---
//
// Caller-supplied identifiers (uniprot_acc, gene, rsid) flow into outbound URLs
// and query strings (UniProt REST, Ensembl VEP, AlphaFold DB file server). To
// prevent constrained SSRF / request-shaping via crafted identifiers, each is
// validated against a strict charset BEFORE it can shape an outbound request.
// Rejections surface as E_PARAMS_INVALID. These guards are intentionally
// conservative but still admit every legitimate symbol the resolvers accept.
var (
	// uniProtAccRe matches a full UniProtKB accession with an optional "-N" isoform
	// suffix, per UniProt's published accession grammar
	// (https://www.uniprot.org/help/accession_numbers):
	//
	//	^([OPQ][0-9][A-Z0-9]{3}[0-9]|[A-NR-Z][0-9]([A-Z0-9]{3}[0-9]){1,2})(-\d+)?$
	//
	// WHY this exact grammar and NOT a naive class like `[A-NR-Z0-9]{6,10}`:
	// O/P/Q-led accessions are a SEPARATE alternation branch ([OPQ][0-9]...). A naive
	// first-position class of [A-NR-Z] excludes O, P and Q entirely, which wrongly
	// rejects P51580 -- this codebase's OWN canonical accession (TPMT) -- as well as
	// Q9Y6K9. The [A-NR-Z][0-9]... branch covers the non-O/P/Q 6- and 10-char forms
	// (e.g. A0A024R161). Do NOT "simplify" this back to a single charset: doing so
	// silently breaks every O/P/Q accession the resolvers actually use.
	uniProtAccRe = regexp.MustCompile(`^([OPQ][0-9][A-Z0-9]{3}[0-9]|[A-NR-Z][0-9]([A-Z0-9]{3}[0-9]){1,2})(-\d+)?$`)
	// geneSymbolRe is a conservative charset guard for HGNC-style gene symbols and
	// transcript-ish tokens: letters, digits, dot, underscore, hyphen.
	geneSymbolRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// rsIDRe matches a dbSNP rsID: "rs" followed by one or more digits.
	rsIDRe = regexp.MustCompile(`^rs\d+$`)
)

// validateUniProtAcc enforces the strict accession grammar on a caller-supplied
// or resolved UniProt accession before it is interpolated into outbound UniProt /
// AlphaFold URLs. Empty is allowed (the caller decides whether an accession is
// required); a non-empty malformed accession is rejected with E_PARAMS_INVALID.
func validateUniProtAcc(acc string) error {
	if acc == "" {
		return nil
	}
	if !uniProtAccRe.MatchString(acc) {
		return newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("uniprot_acc %q is not a valid UniProt accession", acc), nil)
	}
	return nil
}

// validateGene enforces a conservative charset on a gene symbol before it shapes
// an outbound UniProt search query. Empty is allowed (resolution may proceed via
// a pinned accession); a non-empty out-of-charset symbol is rejected.
func validateGene(gene string) error {
	if gene == "" {
		return nil
	}
	if !geneSymbolRe.MatchString(gene) {
		return newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("gene %q contains characters not allowed in a gene symbol", gene), nil)
	}
	return nil
}

// validateRSID enforces the rsID grammar before it shapes an outbound Ensembl VEP
// request. Empty is rejected by the rsid-mode parser separately; here a non-empty
// value must match "rs<digits>".
func validateRSID(rsid string) error {
	if !rsIDRe.MatchString(rsid) {
		return newResolveError(ECodeParamsInvalid,
			fmt.Sprintf("rsid %q is not a valid dbSNP rsID", rsid), nil)
	}
	return nil
}
