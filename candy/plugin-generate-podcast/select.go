package generatepodcast

import (
	"fmt"
	"sort"
	"strings"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// Signals are the named reasons an entry scored what it scored. They are recorded so the coverage
// statement can say WHY something was chosen -- a selection a listener cannot interrogate is a black box.
type Signals struct {
	Source_id string
	Repo      string
	Calver    string
	Title     string
	Score     int
	Signals   []string
}

// Selection is the chosen entries plus everything that was skipped, with its score.
type Selection struct {
	Chosen  []Signals
	Skipped []Signals
}

// The scoring table is FIXED so two runs over one bundle select the same segments. Every weight is a
// deliberate editorial choice, not a heuristic: capability and honest-failure material first, noise last.
const (
	wCapability  = 3  // a new verb/kind/provider/device path: the raw material for C3's "what agents can now do"
	wFailure     = 2  // the body confesses a failure, a refusal, or a "was silently X" -- the best material in the org
	wUserVisible = 2  // a command, an image, a default: C2's stake
	wBlast       = 2  // >= 10 files or >= 3 repos
	wNumber      = 2  // a measured number in the entry: gives the receipt a spine (C4)
	wDocsOnly    = -3 // true, and not a segment
)

// Select scores every entry, takes the top `want`, then applies the diversity constraint (at most two
// from one repo family). The constraint is applied AFTER the ranking so it cannot hide a strong entry --
// it only stops one loud repo from filling the show.
func Select(b params.SourceBundle, want int) (Selection, error) {
	if want < 3 || want > 5 {
		return Selection{}, fmt.Errorf("select: want %d is outside the 3..5 the format allows", want)
	}
	scored := make([]Signals, 0, len(b.Entries))
	for _, e := range b.Entries {
		s := Signals{Source_id: e.Id, Repo: e.Repo, Calver: e.Calver, Title: e.Title}
		low := strings.ToLower(e.Body)
		if hasAny(low, "adds", "new verb", "new kind", "new provider", "new command", "verb:", "provider") {
			s.Score += wCapability
			s.Signals = append(s.Signals, "capability")
		}
		if hasAny(low, "fail", "refus", "silently", "was silently", "error", "regression") {
			s.Score += wFailure
			s.Signals = append(s.Signals, "honest-failure")
		}
		if hasAny(low, "default", "cli", "command", "image", "flag") {
			s.Score += wUserVisible
			s.Signals = append(s.Signals, "user-visible")
		}
		if strings.Count(low, "\n") >= 10 || strings.Count(low, "github.com/opencharly/") >= 3 {
			s.Score += wBlast
			s.Signals = append(s.Signals, "blast-radius")
		}
		if hasNumber(low) {
			s.Score += wNumber
			s.Signals = append(s.Signals, "measured-number")
		}
		if hasAny(low, "docs:", "typo", "readme") && !hasAny(low, "code", "behaviour", "behavior") {
			s.Score += wDocsOnly
			s.Signals = append(s.Signals, "docs-only")
		}
		scored = append(scored, s)
	}
	// deterministic: score desc, then Source_id asc so ties never reorder between runs
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Source_id < scored[j].Source_id
	})

	var sel Selection
	perRepo := map[string]int{}
	for _, s := range scored {
		if len(sel.Chosen) < want && perRepo[s.Repo] < 2 {
			sel.Chosen = append(sel.Chosen, s)
			perRepo[s.Repo]++
			continue
		}
		sel.Skipped = append(sel.Skipped, s)
	}
	return sel, nil
}

func hasAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// hasNumber reports whether the text carries a measured quantity (a digit adjacent to a unit-ish word).
func hasNumber(hay string) bool {
	for i := 0; i < len(hay)-1; i++ {
		if hay[i] >= '0' && hay[i] <= '9' {
			for _, u := range []string{"ms", "kb", "mb", "gb", "%", "x ", " s ", "bytes", "repos", "files"} {
				if strings.Contains(hay[i:], u) {
					return true
				}
			}
		}
	}
	return false
}
