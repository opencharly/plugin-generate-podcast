package generatepodcast

import (
	"strings"
	"testing"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

func sample() params.Episode {
	var ep params.Episode
	ep.Front_matter.Episode = "sample"
	ep.Front_matter.Topic = "a topic"
	ep.Front_matter.Profile = "news-desk"
	ep.Front_matter.Cutoff = "2026.274"
	ep.Front_matter.Sources = 9
	ep.Front_matter.Coverage.Selected, ep.Front_matter.Coverage.Skipped = 4, 592
	ep.Front_matter.Coverage.Repos_scanned, ep.Front_matter.Coverage.Repos_used = 243, 9
	ep.Front_matter.Gaps = []string{"what the window could not answer"}
	ep.Front_matter.Cast = map[string]struct {
		Sid   int64  `json:"sid"`
		Voice string `json:"voice"`
	}{"CONCH": {Sid: 1, Voice: "af_bella"}, "TEMPER": {Sid: 6, Voice: "am_michael"}}
	ep.Cold_open = "the topic, in words a listener can hear"
	for _, t := range []string{"one", "two", "three"} {
		ep.Segments = append(ep.Segments, params.Segment{
			Title: t, Mechanism: "what the sources say about " + t,
			Receipt: "a verbatim passage", Receipt_source: "src-1",
			Unlock: "an agent can now do X", Honest_limit: "this does not establish Y",
			Comic_beat: "the laugh lives here",
		})
	}
	ep.Pinch.Claim, ep.Pinch.Citation = "the one consequence claim", "src-1"
	ep.Close = "point at the receipts"
	return ep
}

// The cue sequence IS the show's structure: enthusiast/mechanism, anchor/receipt, enthusiast/unlock,
// anchor/honest-limit, then a pause. If this drifts, the audio drifts from the script.
func TestEmitCueOrder(t *testing.T) {
	_, cues, err := Emit(sample(), Cast{Enthusiast: "CONCH", Anchor: "TEMPER"})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	var got []string
	for _, c := range cues {
		if c.Speaker == "" {
			got = append(got, "--")
		} else {
			got = append(got, c.Speaker)
		}
	}
	want := []string{
		"CONCH",
		"CONCH", "TEMPER", "CONCH", "TEMPER", "--",
		"CONCH", "TEMPER", "CONCH", "TEMPER", "--",
		"CONCH", "TEMPER", "CONCH", "TEMPER", "--",
		"TEMPER", "CONCH",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cue order drifted:\n got %v\nwant %v", got, want)
	}
}

// Fails without the guard: an unmapped role name must be refused, not rendered as an unknown speaker.
func TestEmitRejectsUncastRole(t *testing.T) {
	if _, _, err := Emit(sample(), Cast{Enthusiast: "CONCH", Anchor: "NOBODY"}); err == nil {
		t.Fatal("expected an error for an anchor that is not in the episode's cast")
	}
	if _, _, err := Emit(sample(), Cast{}); err == nil {
		t.Fatal("expected an error when the cast roles are unnamed")
	}
}

// Fails without the guard: the schema's `[_, _, _]` cannot be expressed in Go, so an empty episode must
// be refused here rather than emitting a plausible-looking empty script.
func TestEmitRejectsEmptyEpisode(t *testing.T) {
	ep := sample()
	ep.Segments = nil
	if _, _, err := Emit(ep, Cast{Enthusiast: "CONCH", Anchor: "TEMPER"}); err == nil {
		t.Fatal("expected an error for an episode with no segments")
	}
}

// The markdown must carry the coverage numbers and the gaps -- the episode's own receipts.
func TestEmitMarkdownCarriesReceipts(t *testing.T) {
	md, _, err := Emit(sample(), Cast{Enthusiast: "CONCH", Anchor: "TEMPER"})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	for _, want := range []string{"coverage: {selected: 4, skipped: 592", "repos_scanned: 243",
		`gaps: ["what the window could not answer"]`, "CONCH: the topic", "TEMPER: a verbatim passage [src-1]"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
}
