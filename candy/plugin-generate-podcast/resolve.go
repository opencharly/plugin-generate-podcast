package generatepodcast

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// calverFile matches a CHANGELOG entry filename: <YYYY>.<DDD>.<HHMM>.md
var calverFile = regexp.MustCompile(`^([0-9]{4}\.[0-9]{3}\.[0-9]{4})\.md$`)

// calverFloor matches a CalVer floor: <YYYY>.<DDD>
var calverFloor = regexp.MustCompile(`^[0-9]{4}\.[0-9]{3}$`)

// ResolveChangelogWindow builds a #SourceBundle from the CHANGELOG entries at or after `cutoff`.
//
// The cutoff is a STRING comparison on `YYYY.DDD` and nothing else -- CalVer sorts lexicographically as
// chronologically, so "since the cutoff" needs no git and no date arithmetic. A `2026.280.0704` entry is
// in a `2026.274` window because "2026.280.0704" >= "2026.274".
//
// `tags` is the tag list per repo (repo -> tags), supplied by the caller so this stays pure and testable:
// the reconciliation below is the point, not the git call that feeds it. A tag with no entry is a real
// gap -- a landing that produced no changelog -- and is reported rather than silently lost.
func ResolveChangelogWindow(root, cutoff string, tags map[string][]string) (params.SourceBundle, error) {
	if !calverFloor.MatchString(cutoff) {
		return params.SourceBundle{}, fmt.Errorf("resolve: cutoff %q is not a CalVer floor (YYYY.DDD)", cutoff)
	}
	repos, err := os.ReadDir(root)
	if err != nil {
		return params.SourceBundle{}, fmt.Errorf("resolve: reading %s: %w", root, err)
	}
	var b params.SourceBundle
	b.Cutoff = cutoff
	b.Generated_at = time.Now().UTC().Format(time.RFC3339)

	have := map[string]bool{} // "repo@calver" -> an entry exists
	for _, r := range repos {
		if !r.IsDir() || strings.HasPrefix(r.Name(), ".") {
			continue
		}
		repo := r.Name()
		files, err := os.ReadDir(filepath.Join(root, repo, "CHANGELOG"))
		if err != nil {
			continue // a repo with no CHANGELOG/ contributes nothing, and is not an error
		}
		b.Repos_scanned++
		n := 0
		for _, f := range files {
			m := calverFile.FindStringSubmatch(f.Name())
			if m == nil || m[1] < cutoff {
				continue // below the floor, or not a CalVer entry
			}
			body, err := os.ReadFile(filepath.Join(root, repo, "CHANGELOG", f.Name()))
			if err != nil {
				continue
			}
			// ONE place computes a digest/byte count/title (resolvers.go's newSource), so a
			// changelog-window receipt and a file/glob/doc receipt are comparable by construction.
			src := newSource("changelog-window", repo+"@"+m[1], repo+"@"+m[1], repo,
				filepath.Join(repo, "CHANGELOG", f.Name()), body)
			src.Calver = m[1]
			b.Entries = append(b.Entries, src)
			have[repo+"@"+m[1]] = true
			n++
		}
		if n > 0 {
			b.Repos_with_entries++
		}
	}
	// reconcile tags against entries: a tag with no entry is a gap the episode must be able to state
	for repo, repoTags := range tags {
		for _, t := range repoTags {
			calver := strings.TrimPrefix(t, "v")
			if calverFile.MatchString(calver+".md") && !have[repo+"@"+calver] {
				b.Tag_gaps = append(b.Tag_gaps, params.TagGap{Repo: repo, Tag: t, Calver: calver})
			}
		}
	}
	sort.Slice(b.Entries, func(i, j int) bool { return b.Entries[i].Id < b.Entries[j].Id })
	sort.Slice(b.Tag_gaps, func(i, j int) bool { return b.Tag_gaps[i].Repo < b.Tag_gaps[j].Repo })
	return b, nil
}

// firstHeading returns the first markdown heading of an entry, or its first non-empty line.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		return strings.TrimSpace(strings.TrimLeft(t, "#"))
	}
	return ""
}
