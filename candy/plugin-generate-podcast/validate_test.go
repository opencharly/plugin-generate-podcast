package generatepodcast

import (
	"strings"
	"testing"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// receiptBundle is a bundle whose first source carries a known passage, so a verbatim quote can be
// distinguished from a paraphrase.
func receiptBundle() params.SourceBundle {
	return params.SourceBundle{
		Cutoff:             "2026.274",
		Generated_at:       "2026-10-08T00:00:00Z",
		Repos_scanned:      3,
		Repos_with_entries: 3,
		Entries: []params.Source{
			{Id: "alpha@2026.280.0704", Repo: "alpha", Calver: "2026.280.0704",
				Title: "adds a new verb", Digest: strings.Repeat("a", 64), Bytes: 120,
				Body: "adds a new verb\nit was silently ignoring fail; the error path is now refused\nmeasured 42 repos\n"},
			{Id: "beta@2026.281.0900", Repo: "beta", Calver: "2026.281.0900",
				Title: "a default", Digest: strings.Repeat("b", 64), Bytes: 90,
				Body: "a new default for the cli\n"},
			{Id: "gamma@2026.279.1200", Repo: "gamma", Calver: "2026.279.1200",
				Title: "an image change", Digest: strings.Repeat("c", 64), Bytes: 70,
				Body: "the image gained a flag\n"},
		},
	}
}

func typedOutputs(quote string) params.TypedOutputs {
	return params.TypedOutputs{
		Script:  "# the episode\nCONCH: a verb landed\n",
		Sources: []params.Claim{{Claim_id: "c1", Source_id: "alpha@2026.280.0704", Quote: quote}},
		Coverage: params.Coverage{
			Selected: 1, Skipped: 2, Repos_scanned: 3, Repos_used: 1,
		},
		Gaps: []string{},
	}
}

// The happy path: a quote copied out of the cited source passes.
func TestValidateOutputsAcceptsVerbatimQuote(t *testing.T) {
	if err := ValidateOutputs(typedOutputs("it was silently ignoring fail"), receiptBundle(), nil); err != nil {
		t.Fatalf("a verbatim quote was refused: %v", err)
	}
}

// Fails without the guard -- this IS the anti-fabrication gate (C4): a plausible paraphrase of a real
// entry must not pass as a receipt.
func TestValidateOutputsRejectsFabricatedQuote(t *testing.T) {
	err := ValidateOutputs(typedOutputs("the loader now makes agents unstoppable"), receiptBundle(), nil)
	if err == nil {
		t.Fatal("a quote that appears nowhere in the cited source was accepted")
	}
	if !strings.Contains(err.Error(), "verbatim") {
		t.Errorf("the refusal should name the verbatim rule, got %v", err)
	}
}

// Fails without the guard: citing an id the bundle does not hold is a receipt pointing at nothing.
func TestValidateOutputsRejectsUnknownCitation(t *testing.T) {
	out := typedOutputs("adds a new verb")
	out.Sources[0].Source_id = "delta@2026.281.1300"
	if err := ValidateOutputs(out, receiptBundle(), nil); err == nil {
		t.Fatal("a citation that is not in the bundle was accepted")
	}
	// an episode with no receipts at all is refused too (§5.4)
	out = typedOutputs("adds a new verb")
	out.Sources = nil
	if err := ValidateOutputs(out, receiptBundle(), nil); err == nil {
		t.Fatal("a source-free episode was accepted")
	}
	if err := ValidateOutputs(params.TypedOutputs{}, receiptBundle(), nil); err == nil {
		t.Fatal("an empty typed-outputs document was accepted")
	}
}

// Fails without the guard: the coverage beat states what was read, so numbers that do not match the
// bundle are a lie rather than a rounding difference.
func TestValidateOutputsRejectsCoverageThatDoesNotMatch(t *testing.T) {
	out := typedOutputs("adds a new verb")
	out.Coverage.Skipped = 1 // the bundle holds 3 and 1 is selected
	if err := ValidateOutputs(out, receiptBundle(), nil); err == nil {
		t.Fatal("coverage.skipped that contradicts the bundle was accepted")
	}
	out = typedOutputs("adds a new verb")
	out.Coverage.Repos_scanned = 9
	if err := ValidateOutputs(out, receiptBundle(), nil); err == nil {
		t.Fatal("coverage.repos_scanned that contradicts the bundle was accepted")
	}
	out = typedOutputs("adds a new verb")
	out.Coverage.Selected = 0
	if err := ValidateOutputs(out, receiptBundle(), nil); err == nil {
		t.Fatal("a show that selected nothing was accepted")
	}
}

// With a selection in hand the coverage must match the selection itself, not just the arithmetic.
func TestValidateOutputsChecksAgainstSelection(t *testing.T) {
	sel := &Selection{
		Chosen:  []Signals{{Source_id: "alpha@2026.280.0704", Repo: "alpha"}},
		Skipped: []Signals{{Source_id: "beta@2026.281.0900", Repo: "beta"}, {Source_id: "gamma@2026.279.1200", Repo: "gamma"}},
	}
	if err := ValidateOutputs(typedOutputs("adds a new verb"), receiptBundle(), sel); err != nil {
		t.Fatalf("coverage matching the selection was refused: %v", err)
	}
	out := typedOutputs("adds a new verb")
	out.Coverage.Repos_used = 2 // the selection draws on one repo
	if err := ValidateOutputs(out, receiptBundle(), sel); err == nil {
		t.Fatal("coverage.repos_used that contradicts the selection was accepted")
	}
	out = typedOutputs("adds a new verb")
	out.Coverage.Skipped = 1
	if err := ValidateOutputs(out, receiptBundle(), sel); err == nil {
		t.Fatal("coverage.skipped that contradicts the selection was accepted")
	}
}

// A tag with no CHANGELOG entry is a landing the record does not narrate. Fails without the guard: the
// episode claimed a fully covered window while the bundle held a gap.
func TestValidateOutputsRequiresTheGapBeStated(t *testing.T) {
	b := receiptBundle()
	b.Tag_gaps = []params.TagGap{{Repo: "alpha", Tag: "v2026.281.0829", Calver: "2026.281.0829"}}
	if err := ValidateOutputs(typedOutputs("adds a new verb"), b, nil); err == nil {
		t.Fatal("an empty gaps list was accepted while the window held a tag gap")
	}
	out := typedOutputs("adds a new verb")
	out.Gaps = []string{"v2026.281.0829 is a tag with no CHANGELOG file"}
	if err := ValidateOutputs(out, b, nil); err != nil {
		t.Fatalf("a stated gap was refused: %v", err)
	}
}

// The renderer's input gate (S3): an episode whose citations do not resolve in the bundle it was written
// against must not reach the emitter, and the source count must be the bundle's own.
func TestValidateEpisodeInputs(t *testing.T) {
	b := receiptBundle()
	ep := sample()
	ep.Front_matter.Sources = 3
	for i := range ep.Segments {
		ep.Segments[i].Receipt_source = "beta@2026.281.0900"
	}
	if err := ValidateEpisodeInputs(ep, b); err != nil {
		t.Fatalf("a resolvable episode was refused: %v", err)
	}
	ep.Segments[1].Receipt_source = "src-1" // the sample's own placeholder, absent from the bundle
	if err := ValidateEpisodeInputs(ep, b); err == nil {
		t.Fatal("a segment citing a source outside the bundle was accepted")
	}
	ep = sample()
	ep.Front_matter.Sources = 9 // the bundle holds 3
	if err := ValidateEpisodeInputs(ep, b); err == nil {
		t.Fatal("a source count that does not match the bundle was accepted")
	}
	if err := ValidateEpisodeInputs(sample(), params.SourceBundle{}); err == nil {
		t.Fatal("an empty bundle was accepted as the input gate's subject")
	}
}
