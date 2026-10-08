package generatepodcast

import (
	"errors"
	"fmt"
	"strings"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// validate.go — the gates a CUE type cannot express. #Episode and #TypedOutputs prove a shape is
// COMPLETE; what is left is the CROSS-REFERENCE between the episode and the bundle it claims to have
// read. That is the anti-fabrication gate (C4): a quote is only a receipt if it is verbatim in the
// source the claim cites, and a coverage number is only honest if it matches the selection.

// ValidateOutputs is the writing stage's gate (§12.6 semantics + §13.8's typed outputs). It is
// deliberately loud on the two failures the genre actually produces: an invented quotation, and a
// coverage statement that does not match what was read.
func ValidateOutputs(out params.TypedOutputs, b params.SourceBundle, sel *Selection) error {
	if strings.TrimSpace(out.Script) == "" {
		return errors.New("validate: the script is empty")
	}
	if len(out.Sources) == 0 {
		return errors.New("validate: no claim carries a receipt -- a source-free episode is refused (§5.4)")
	}
	if len(b.Entries) == 0 {
		return errors.New("validate: the bundle holds no sources; there is nothing to validate against (§5.4)")
	}
	byID := make(map[string]params.Source, len(b.Entries))
	for _, s := range b.Entries {
		byID[s.Id] = s
	}
	for _, c := range out.Sources {
		src, ok := byID[c.Source_id]
		if !ok {
			return fmt.Errorf("validate: claim %s cites source %q, which is not in the bundle (%d sources; the id IS the citation)",
				c.Claim_id, c.Source_id, len(b.Entries))
		}
		if !strings.Contains(src.Body, c.Quote) {
			return fmt.Errorf("validate: claim %s quotes text that does not appear verbatim in %s -- the anti-fabrication gate (C4)",
				c.Claim_id, c.Source_id)
		}
	}
	return validateCoverage(out, b, sel)
}

// validateCoverage checks the coverage block against the bundle and, when the selection is supplied,
// against the selection the writer was handed. Without a selection the only honest relation is the
// arithmetic one: every entry is either selected or skipped.
func validateCoverage(out params.TypedOutputs, b params.SourceBundle, sel *Selection) error {
	total := int64(len(b.Entries))
	cov := out.Coverage
	if cov.Selected <= 0 || cov.Selected > total {
		return fmt.Errorf("validate: coverage.selected is %d and the bundle holds %d sources -- a show that selected nothing is not a show", cov.Selected, total)
	}
	if sel != nil {
		if cov.Selected != int64(len(sel.Chosen)) {
			return fmt.Errorf("validate: coverage.selected is %d and the selection holds %d chosen -- the published coverage must match the selection it was built from",
				cov.Selected, len(sel.Chosen))
		}
		if cov.Skipped != int64(len(sel.Skipped)) {
			return fmt.Errorf("validate: coverage.skipped is %d and the selection records %d skipped -- the skipped list is what the coverage statement publishes",
				cov.Skipped, len(sel.Skipped))
		}
		used := map[string]bool{}
		for _, c := range sel.Chosen {
			if c.Repo != "" {
				used[c.Repo] = true
			}
		}
		if cov.Repos_used != int64(len(used)) {
			return fmt.Errorf("validate: coverage.repos_used is %d and the selection draws on %d repo(s)",
				cov.Repos_used, len(used))
		}
	} else if cov.Skipped != total-cov.Selected {
		return fmt.Errorf("validate: coverage.skipped is %d; with %d sources and %d selected the honest number is %d",
			cov.Skipped, total, cov.Selected, total-cov.Selected)
	}
	if b.Repos_scanned > 0 && cov.Repos_scanned != b.Repos_scanned {
		return fmt.Errorf("validate: coverage.repos_scanned is %d and the bundle scanned %d -- the clock beat states what was read",
			cov.Repos_scanned, b.Repos_scanned)
	}
	if cov.Repos_scanned > 0 && (cov.Repos_used <= 0 || cov.Repos_used > cov.Repos_scanned) {
		return fmt.Errorf("validate: coverage.repos_used is %d against %d repos scanned", cov.Repos_used, cov.Repos_scanned)
	}
	// A tag with no entry is a landing the record does not narrate. It is material, so the episode has to
	// say it out loud rather than report a fully covered window.
	if len(b.Tag_gaps) > 0 && len(out.Gaps) == 0 {
		return fmt.Errorf("validate: the window holds %d tag gap(s) (a landing with no CHANGELOG entry) and the episode states none -- gaps: is mandatory whenever the window is incomplete",
			len(b.Tag_gaps))
	}
	return nil
}

// ValidateEpisodeInputs is the RENDERER's input gate (S3), and the `emit` path runs it whenever it is
// given the bundle (`--bundle`): that path then refuses to produce cue lines from an episode whose
// citations do not resolve in the bundle it was written against. The renderer consumes the cue lines this
// emitter produces, so validating there is what removes the class of defect where a synthesiser receives
// a script quoting sources nobody can find. `Emit` itself stays bundle-free -- it is the pure
// episode-to-cues function -- so a caller that omits `--bundle` gets the structural checks only.
func ValidateEpisodeInputs(ep params.Episode, b params.SourceBundle) error {
	if len(b.Entries) == 0 {
		return errors.New("emit: the bundle holds no sources; there is nothing to emit against (§5.4)")
	}
	byID := make(map[string]bool, len(b.Entries))
	for _, s := range b.Entries {
		byID[s.Id] = true
	}
	for i, s := range ep.Segments {
		if !byID[s.Receipt_source] {
			return fmt.Errorf("emit: segment %d (%q) names receipt_source %q, which is not in the bundle",
				i+1, s.Title, s.Receipt_source)
		}
	}
	if ep.Front_matter.Sources != int64(len(b.Entries)) {
		return fmt.Errorf("emit: the episode's front matter claims %d sources and the bundle holds %d -- a receipt count that does not match is a lie",
			ep.Front_matter.Sources, len(b.Entries))
	}
	return nil
}
