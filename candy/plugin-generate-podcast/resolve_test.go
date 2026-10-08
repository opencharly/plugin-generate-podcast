package generatepodcast

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// a temp umbrella: three repos, five entries, one of them BELOW the floor.
func tree(t *testing.T, cutoff string) string {
	t.Helper()
	root := t.TempDir()
	write := func(repo, calver, body string) {
		d := filepath.Join(root, repo, "CHANGELOG")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, calver+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("alpha", "2026.270.0100", "# below the floor\nshould not appear\n")
	write("alpha", "2026.280.0704", "# alpha adds a new verb\nprovider: x\nmeasured 42 repos\n")
	write("beta", "2026.281.0900", "# beta docs: a typo in README\n")
	write("gamma", "2026.279.1200", "# gamma was silently ignoring fail\nerror path fixed\n")
	return root
}

// The cutoff is a STRING comparison on YYYY.DDD: an entry below the floor is excluded, one above included.
func TestResolveFiltersBelowFloor(t *testing.T) {
	b, err := ResolveChangelogWindow(tree(t, "2026.274"), "2026.274", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(b.Entries) != 3 {
		t.Fatalf("want 3 entries at or after 2026.274, got %d: %v", len(b.Entries), ids(b))
	}
	for _, e := range b.Entries {
		if e.Calver < "2026.274" {
			t.Errorf("entry %s is below the floor", e.Id)
		}
		if len(e.Digest) != 64 || e.Bytes == 0 || e.Resolver != params.Resolver("changelog-window") {
			t.Errorf("entry %s is not a usable receipt: %+v", e.Id, e)
		}
	}
	if b.Repos_scanned != 3 || b.Repos_with_entries != 3 {
		t.Errorf("scan counts: scanned=%d with_entries=%d", b.Repos_scanned, b.Repos_with_entries)
	}
}

// A tag with no CHANGELOG entry is a real gap (a landing that produced no changelog) and must be reported.
func TestResolveReconcilesTagGaps(t *testing.T) {
	tags := map[string][]string{"alpha": {"v2026.280.0704", "v2026.281.0829"}}
	b, err := ResolveChangelogWindow(tree(t, "2026.274"), "2026.274", tags)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(b.Tag_gaps) != 1 || b.Tag_gaps[0].Tag != "v2026.281.0829" {
		t.Fatalf("want exactly the orphan tag v2026.281.0829 as a gap, got %+v", b.Tag_gaps)
	}
	if b.Tag_gaps[0].Repo != "alpha" || b.Tag_gaps[0].Calver != "2026.281.0829" {
		t.Errorf("gap is missing its repo/calver: %+v", b.Tag_gaps[0])
	}
}

// Fails without the guard: a floor that is not a CalVer is refused rather than silently matching everything.
func TestResolveRejectsBadCutoff(t *testing.T) {
	if _, err := ResolveChangelogWindow(tree(t, "2026.274"), "2026-10-01", nil); err == nil {
		t.Fatal("expected an error for a non-CalVer cutoff")
	}
}

func ids(b params.SourceBundle) []string {
	out := make([]string, 0, len(b.Entries))
	for _, e := range b.Entries {
		out = append(out, e.Id)
	}
	return out
}

func bundle() params.SourceBundle {
	return params.SourceBundle{Entries: []params.Source{
		{Id: "alpha@2026.280.0704", Repo: "alpha", Calver: "2026.280.0704", Title: "adds a new verb",
			Body: "this adds a new verb\nmeasured 42 repos\nprovider: x\nblah\nblah\nblah\nblah\nblah\nblah\nblah\nblah\n"},
		{Id: "beta@2026.281.0900", Repo: "beta", Calver: "2026.281.0900", Title: "docs: typo",
			Body: "docs: a typo in README\n"},
		{Id: "gamma@2026.279.1200", Repo: "gamma", Calver: "2026.279.1200", Title: "was silently ignoring fail",
			Body: "it was silently ignoring fail; the error path is now refused\n"},
	}}
}

// Deterministic and ranked. NOTE: with exactly `want` entries NOTHING is skipped -- selection excludes
// only by RANKING, so the exclusion case needs more entries than `want`. (Asserting a skip at
// want == len(entries) was the author's error, not the code's.)
func TestSelectRanksAndIsDeterministic(t *testing.T) {
	b := bundle()
	b.Entries = append(b.Entries,
		params.Source{Id: "delta@2026.281.1200", Repo: "delta", Calver: "2026.281.1200",
			Body: "adds a new verb and fixes a refusal\nprovider: y\nmeasured 7 files\n"},
		params.Source{Id: "epsilon@2026.281.1300", Repo: "epsilon", Calver: "2026.281.1300",
			Body: "a new command default, 3 repos touched\n"})
	first, err := Select(b, 3)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	again, _ := Select(b, 3)
	if len(first.Chosen) != len(again.Chosen) {
		t.Fatalf("selection is not stable between runs")
	}
	for i := range first.Chosen {
		if first.Chosen[i].Source_id != again.Chosen[i].Source_id {
			t.Fatalf("order changed between runs: %v vs %v", first.Chosen[i], again.Chosen[i])
		}
	}
	if first.Chosen[0].Source_id != "alpha@2026.280.0704" {
		t.Errorf("capability entry should rank first, got %v", first.Chosen[0])
	}
	var sawDocsInSkipped bool
	for _, s := range first.Skipped {
		if s.Source_id == "beta@2026.281.0900" {
			sawDocsInSkipped = true
		}
		for _, sig := range s.Signals {
			if sig == "docs-only" && s.Score >= 0 {
				t.Errorf("a docs-only entry scored %d; the weight must make it unattractive", s.Score)
			}
		}
	}
	if !sawDocsInSkipped {
		t.Errorf("the docs-only entry should rank below `want` with 5 entries for 3 slots: %+v", first)
	}
}

// Fails without the constraint: one loud repo must not fill the show.
func TestSelectEnforcesDiversity(t *testing.T) {
	b := bundle()
	for _, c := range []string{"2026.281.1000", "2026.281.1100"} {
		b.Entries = append(b.Entries, params.Source{Id: "alpha@" + c, Repo: "alpha", Calver: c,
			Body: "adds a new verb\nprovider: x\nmeasured 9 repos\n"})
	}
	sel, err := Select(b, 3)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	n := 0
	for _, c := range sel.Chosen {
		if c.Repo == "alpha" {
			n++
		}
	}
	if n > 2 {
		t.Errorf("diversity constraint broken: %d entries from alpha", n)
	}
}

// Fails without the guard: `want` outside the format's 3..5 is refused.
func TestSelectRejectsOutOfRangeWant(t *testing.T) {
	if _, err := Select(bundle(), 9); err == nil {
		t.Fatal("expected an error for want=9")
	}
}
