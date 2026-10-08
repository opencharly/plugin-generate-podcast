// Package generatepodcast holds the podcast desk's emitter.
//
// The episode is DATA. `cue vet` validates the authored CUE against #Episode; `cue export` yields the
// JSON that decodes into the GENERATED types in ./params (never hand-transcribed). This package turns
// that validated data into (a) the episode markdown and (b) the cue lines `podcast-render` consumes —
// so a malformed episode cannot reach the synthesiser: it cannot be constructed.
package generatepodcast

import (
	"errors"
	"fmt"
	"strings"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// Cast names the two ROLES the show is built from. The names are the caller's — nothing here hardcodes
// a host name, so a profile may cast any pair (the news desk's own pair is CONCH/TEMPER).
type Cast struct {
	Enthusiast string // speaks the mechanism and the unlock
	Anchor     string // speaks the receipt and the honest limit; wins every factual exchange
}

// Cue is one thing the renderer says. An empty Speaker is a pause, written as a bare `--`.
type Cue struct {
	Speaker string
	Text    string
}

func (c Cue) String() string {
	if c.Speaker == "" {
		return "--"
	}
	return c.Speaker + ": " + c.Text
}

// Emit validates what the generated TYPE cannot express and returns the markdown plus the cue lines.
//
// What is deliberately NOT re-checked here: the required fields, the beats and the 3..5 segment count are
// the SCHEMA's job, enforced by `cue vet` before this ever runs — re-implementing them in Go would be a
// hand-transcribed second copy of the schema. The two guards below are the type-inexpressible remainder,
// and they fail loudly rather than emitting a plausible-looking empty script.
func Emit(ep params.Episode, cast Cast) (string, []Cue, error) {
	if cast.Enthusiast == "" || cast.Anchor == "" {
		return "", nil, errors.New("emit: both cast roles must be named")
	}
	if _, ok := ep.Front_matter.Cast[cast.Enthusiast]; !ok {
		return "", nil, fmt.Errorf("emit: enthusiast %q is not in the episode's cast", cast.Enthusiast)
	}
	if _, ok := ep.Front_matter.Cast[cast.Anchor]; !ok {
		return "", nil, fmt.Errorf("emit: anchor %q is not in the episode's cast", cast.Anchor)
	}
	if len(ep.Segments) == 0 {
		// #Episode declares `[_, _, _]`; Go cannot express that, so guard it here rather than emit "".
		return "", nil, errors.New("emit: the episode has no segments")
	}

	var cues []Cue
	say := func(speaker, text string) {
		if text == "" {
			return
		}
		cues = append(cues, Cue{Speaker: speaker, Text: text})
	}

	say(cast.Enthusiast, ep.Cold_open)
	for _, s := range ep.Segments {
		say(cast.Enthusiast, s.Mechanism)
		say(cast.Anchor, fmt.Sprintf("%s [%s]", s.Receipt, s.Receipt_source))
		say(cast.Enthusiast, string(s.Unlock))
		say(cast.Anchor, string(s.Honest_limit))
		cues = append(cues, Cue{}) // a pause between segments
	}
	if ep.Pinch.Claim != "" {
		say(cast.Anchor, fmt.Sprintf("%s [%s]", ep.Pinch.Claim, ep.Pinch.Citation))
	}
	say(cast.Enthusiast, ep.Close)

	return markdown(ep, cues, cast), cues, nil
}

// markdown renders the episode as the published artifact. It is built FROM the cue lines, so the script a
// listener reads and the audio a listener hears are the same content in two substrates.
func markdown(ep params.Episode, cues []Cue, cast Cast) string {
	fm := ep.Front_matter
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "episode: %s\n", fm.Episode)
	fmt.Fprintf(&b, "topic: %s\n", fm.Topic)
	fmt.Fprintf(&b, "profile: %s\n", fm.Profile)
	fmt.Fprintf(&b, "cutoff: %q\n", fm.Cutoff)
	fmt.Fprintf(&b, "cast: {")
	names := make([]string, 0, len(fm.Cast))
	for n := range fm.Cast {
		names = append(names, n)
	}
	sortStrings(names)
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: {sid: %d, voice: %s}", n, fm.Cast[n].Sid, fm.Cast[n].Voice)
	}
	b.WriteString("}\n")
	fmt.Fprintf(&b, "sources: %d\n", fm.Sources)
	fmt.Fprintf(&b, "coverage: {selected: %d, skipped: %d, repos_scanned: %d, repos_used: %d}\n",
		fm.Coverage.Selected, fm.Coverage.Skipped, fm.Coverage.Repos_scanned, fm.Coverage.Repos_used)
	fmt.Fprintf(&b, "gaps: %s\n", quoteList(fm.Gaps))
	b.WriteString("---\n\n")

	b.WriteString("## Cold open\n")
	if len(cues) > 0 {
		fmt.Fprintf(&b, "%s\n", cues[0].String())
	}
	for i, s := range ep.Segments {
		fmt.Fprintf(&b, "\n## Segment %d — %s\n", i+1, s.Title)
		fmt.Fprintf(&b, "%s: %s\n", cast.Enthusiast, s.Mechanism)
	}
	// the segment bodies are already cue lines; render them verbatim so the two substrates cannot diverge
	b.WriteString("\n<!-- spoken track (identical to the renderer's cues) -->\n")
	for _, c := range cues {
		fmt.Fprintf(&b, "%s\n", c.String())
	}
	return b.String()
}

func quoteList(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, fmt.Sprintf("%q", x))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}
