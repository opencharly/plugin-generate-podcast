package generatepodcast

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// The published vocabulary and the adapter table are ONE closed set. Fails without the guard: a word in
// schema/source.cue with no adapter is a published claim nobody serves, and an adapter outside the
// vocabulary is a capability the schema refuses to author.
func TestResolversMatchSchemaVocabulary(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("schema", "source.cue"))
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	var declared []string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#Resolver:") {
			continue
		}
		for _, part := range strings.Split(strings.SplitN(line, ":", 2)[1], "|") {
			declared = append(declared, strings.Trim(strings.TrimSpace(part), `"`))
		}
	}
	if len(declared) == 0 {
		t.Fatal("no #Resolver vocabulary found in schema/source.cue")
	}
	have := map[string]bool{}
	for _, n := range ResolverNames() {
		have[n] = true
	}
	for _, want := range declared {
		if !have[want] {
			t.Errorf("the schema declares resolver %q and no adapter implements it", want)
		}
		delete(have, want)
	}
	for extra := range have {
		t.Errorf("adapter %q is implemented but the #Resolver vocabulary does not declare it", extra)
	}
}

func TestGlobRegexp(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"src/**/*.go", "src/a.go", true}, // `**` matches zero directories
		{"src/**/*.go", "src/deep/b.go", true},
		{"src/**/*.go", "src/deep/er/c.go", true},
		{"src/**/*.go", "src/a.md", false},
		{"src/**/*.go", "other/a.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false}, // `*` stays inside one segment
		{"docs/?.md", "docs/a.md", true},
		{"docs/?.md", "docs/ab.md", false},
		{"CHANGELOG/2026.*.md", "CHANGELOG/2026.280.0704.md", true},
	}
	for _, c := range cases {
		re, err := globRegexp(c.pattern)
		if err != nil {
			t.Fatalf("globRegexp(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.name); got != c.want {
			t.Errorf("glob %q against %q = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// resolverRoot builds a small checkout: one repo with a source tree and a document, a doctrine root, and
// a prior episode.
func resolverRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("alpha/README.md", "# alpha adds a new verb\n\nwhat it does\n")
	mk("alpha/src/a.go", "// a new verb lives here\npackage alpha\n")
	mk("alpha/src/deep/b.go", "package deep\n\nfunc B() {}\n")
	mk("alpha/docs/README.md", "# alpha docs\n")
	mk("alpha/empty.go", "\n")
	mk("notes.md", "# desk notes\n\nwhat the desk read\n")
	mk("AGENTS.md", "# the rulebook\n\nthe rules\n")
	mk("charly/AGENTS.md", "# the core rulebook\n\ncore rules\n")
	mk("SOUL.md", "# soul\n")
	mk("VISION.md", "# vision\n")
	mk("GRIEVANCES.md", "# grievances\n")
	mk("news-podcast/episodes/2026-10-01-prior.md", "# prior episode\n\nCONCH: we shipped a thing\n")
	return root
}

func resolveOne(t *testing.T, root, raw string, d Deps) (params.SourceBundle, error) {
	t.Helper()
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec(%q): %v", raw, err)
	}
	d.Root = root
	return Resolve([]Spec{spec}, d)
}

func TestResolveFile(t *testing.T) {
	root := resolverRoot(t)
	b, err := resolveOne(t, root, "file:alpha@HEAD:README.md", Deps{})
	if err != nil {
		t.Fatalf("file resolver: %v", err)
	}
	if len(b.Entries) != 1 {
		t.Fatalf("want one source, got %d", len(b.Entries))
	}
	s := b.Entries[0]
	if s.Resolver != params.Resolver("file") || s.Repo != "alpha" || s.Path != "README.md" {
		t.Errorf("source is missing its provenance: %+v", s)
	}
	if len(s.Digest) != 64 || s.Bytes == 0 || s.Title != "alpha adds a new verb" {
		t.Errorf("source is not a usable receipt: %+v", s)
	}
	// the id carries the resolver, so a file and a glob over one path cannot collide as citations
	if s.Id != "file:alpha@HEAD:README.md" {
		t.Errorf("file source id is not <resolver>:<ref>: %q", s.Id)
	}
	// a missing file is a named failure, never an empty bundle
	if _, err := resolveOne(t, root, "file:alpha@HEAD:nope.go", Deps{}); err == nil {
		t.Error("expected an error for a missing file")
	}
	// an empty file is refused: bytes > 0 is part of being a receipt
	if _, err := resolveOne(t, root, "file:alpha@HEAD:empty.go", Deps{}); err == nil {
		t.Error("expected an error for an empty file")
	}
}

func TestResolveGlob(t *testing.T) {
	root := resolverRoot(t)
	b, err := resolveOne(t, root, "glob:alpha@HEAD:src/**/*.go", Deps{})
	if err != nil {
		t.Fatalf("glob resolver: %v", err)
	}
	if len(b.Entries) != 2 {
		t.Fatalf("want the two .go files, got %d: %v", len(b.Entries), ids(b))
	}
	if ids(b)[0] != "glob:alpha@HEAD:src/a.go" || ids(b)[1] != "glob:alpha@HEAD:src/deep/b.go" {
		t.Errorf("ids are not stable and sorted: %v", ids(b))
	}
	// a pattern that matches nothing is refused, not answered with an empty set
	if _, err := resolveOne(t, root, "glob:alpha@HEAD:src/**/*.rs", Deps{}); err == nil {
		t.Error("expected an error for a pattern that matches nothing")
	}
}

func TestResolveDoc(t *testing.T) {
	root := resolverRoot(t)
	b, err := resolveOne(t, root, "doc:notes.md", Deps{})
	if err != nil {
		t.Fatalf("doc resolver (bare path): %v", err)
	}
	if b.Entries[0].Title != "desk notes" || b.Entries[0].Resolver != params.Resolver("doc") {
		t.Errorf("doc source is wrong: %+v", b.Entries[0])
	}
	b, err = resolveOne(t, root, "doc:file://"+filepath.Join(root, "notes.md"), Deps{})
	if err != nil {
		t.Fatalf("doc resolver (file://): %v", err)
	}
	if b.Entries[0].Path != filepath.Join(root, "notes.md") {
		t.Errorf("file:// path was not kept: %+v", b.Entries[0])
	}
	// http is exercised against a REAL local server, never a canned body
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# fetched\n\nthe live document\n"))
	}))
	defer srv.Close()
	b, err = resolveOne(t, root, "doc:"+srv.URL+"/doc.md", Deps{})
	if err != nil {
		t.Fatalf("doc resolver (http): %v", err)
	}
	if b.Entries[0].Title != "fetched" {
		t.Errorf("fetched document is wrong: %+v", b.Entries[0])
	}
	// pdf: names the missing prerequisite instead of degrading into text
	if _, err := resolveOne(t, root, "doc:pdf:manual.pdf", Deps{}); err == nil {
		t.Error("expected a named error for pdf: without pdftotext")
	}
}

func TestResolveDoctrine(t *testing.T) {
	root := resolverRoot(t)
	b, err := resolveOne(t, root, "doctrine:profile:org-rules", Deps{})
	if err != nil {
		t.Fatalf("doctrine resolver: %v", err)
	}
	if len(b.Entries) != 2 || b.Entries[0].Id != "doctrine:org-rules:AGENTS.md" {
		t.Fatalf("org-rules should hold the two rulebooks, got %v", ids(b))
	}
	if _, err := resolveOne(t, root, "doctrine:profile:nope", Deps{}); err == nil {
		t.Error("expected an error for an unknown doctrine profile")
	}
	if _, err := resolveOne(t, root, "doctrine:org-rules", Deps{}); err == nil {
		t.Error("expected an error for a ref that is not profile:<name>")
	}
	// a named set is a promise about its members: a missing one is a failure
	if err := os.Remove(filepath.Join(root, "charly/AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOne(t, root, "doctrine:profile:org-rules", Deps{}); err == nil {
		t.Error("expected an error when a named doctrine member is missing")
	}
}

func TestResolveEpisode(t *testing.T) {
	root := resolverRoot(t)
	b, err := resolveOne(t, root, "episode:news-podcast:episodes/2026-10-01-prior.md", Deps{})
	if err != nil {
		t.Fatalf("episode resolver: %v", err)
	}
	if b.Entries[0].Id != "episode:news-podcast:episodes/2026-10-01-prior.md" || b.Entries[0].Repo != "news-podcast" {
		t.Errorf("episode source is wrong: %+v", b.Entries[0])
	}
	if _, err := resolveOne(t, root, "episode:news-podcast", Deps{}); err == nil {
		t.Error("expected an error for a ref with no episode path")
	}
}

func TestResolveGitLog(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; the live arm is the R10 bed's, this unit test needs a real repo")
	}
	root := resolverRoot(t)
	repo := filepath.Join(root, "alpha")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", "src")
	run("commit", "-q", "-m", "adds a new verb")
	// the second landing MUST touch the filtered path: `git log <range> -- <path>` uses history
	// simplification, so an empty commit is not a commit "under src" and would (correctly) not appear.
	if err := os.WriteFile(filepath.Join(repo, "src", "c.go"), []byte("// a second verb\npackage alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "src/c.go")
	run("commit", "-q", "-m", "the second landing")

	b, err := resolveOne(t, root, "git-log:alpha@HEAD~1..HEAD:src", Deps{})
	if err != nil {
		t.Fatalf("git-log resolver: %v", err)
	}
	if len(b.Entries) != 1 || b.Entries[0].Title != "the second landing" {
		t.Fatalf("want the one commit in the range, got %v", ids(b))
	}
	if !strings.HasPrefix(b.Entries[0].Id, "git-log:alpha@") || len(b.Entries[0].Digest) != 64 {
		t.Errorf("git-log source is not a receipt: %+v", b.Entries[0])
	}
	// the range is real: a range with no commits is refused
	if _, err := resolveOne(t, root, "git-log:alpha@HEAD..HEAD:src", Deps{}); err == nil {
		t.Error("expected an error for an empty range")
	}
	// a missing repo directory surfaces git's own refusal
	if _, err := resolveOne(t, root, "git-log:gamma@HEAD:src", Deps{}); err == nil {
		t.Error("expected an error for a repo that is not a git checkout")
	}
}

// ghServer answers the two REST shapes the issues/prs resolvers read. It is a real HTTP server on a
// loopback port -- the resolvers' own boundary, not a canned in-process answer.
func ghServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/opencharly/plugin-generate-podcast/issues", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"number": 7, "title": "the desk loses a receipt", "body": "measured on the 2026.274 window"},
			{"number": 8, "title": "a pull request, not an issue", "body": "ignored", "pull_request": map[string]any{"url": "x"}},
		})
	})
	mux.HandleFunc("/repos/opencharly/plugin-generate-podcast/pulls", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"number": 3, "title": "the emitter lands", "merged_at": "2026-10-08T12:00:00Z"},
			{"number": 2, "title": "too old for the window", "merged_at": "2026-09-01T12:00:00Z"},
		})
	})
	mux.HandleFunc("/repos/opencharly/plugin-generate-podcast/pulls/3", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 3, "title": "the emitter lands", "body": "the PR body IS the changelog",
			"merged_at": "2026-10-08T12:00:00Z", "additions": 120, "deletions": 4,
			"changed_files": 3, "html_url": "https://example.invalid/3",
		})
	})
	mux.HandleFunc("/repos/opencharly/nope/issues", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/repos/opencharly/limited/issues", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})
	return httptest.NewServer(mux)
}

func TestResolveIssuesAndPRs(t *testing.T) {
	root := resolverRoot(t)
	srv := ghServer(t)
	defer srv.Close()
	d := Deps{APIBase: srv.URL}

	b, err := resolveOne(t, root, "issues:opencharly/plugin-generate-podcast?state=all&limit=5", d)
	if err != nil {
		t.Fatalf("issues resolver: %v", err)
	}
	if len(b.Entries) != 1 || b.Entries[0].Id != "issues:opencharly/plugin-generate-podcast#7" {
		t.Fatalf("the issues endpoint also returns PRs; want exactly issue 7, got %v", ids(b))
	}
	if !strings.Contains(b.Entries[0].Body, "measured on the 2026.274 window") {
		t.Errorf("the issue body is not carried: %q", b.Entries[0].Body)
	}

	b, err = resolveOne(t, root, "prs:opencharly/plugin-generate-podcast?merged-since=2026-10-01&limit=5", d)
	if err != nil {
		t.Fatalf("prs resolver: %v", err)
	}
	if len(b.Entries) != 1 || b.Entries[0].Id != "prs:opencharly/plugin-generate-podcast#3" {
		t.Fatalf("want only the PR merged inside the window, got %v", ids(b))
	}
	if !strings.Contains(b.Entries[0].Body, "+120/-4 across 3 file(s)") {
		t.Errorf("the diffstat is missing from the PR receipt: %q", b.Entries[0].Body)
	}

	// the two refusals a live API answers with are named, not swallowed
	if _, err := resolveOne(t, root, "issues:opencharly/nope?state=all", d); err == nil ||
		!strings.Contains(err.Error(), "404") {
		t.Errorf("want a named 404 refusal, got %v", err)
	}
	if _, err := resolveOne(t, root, "issues:opencharly/limited?state=all", d); err == nil ||
		!strings.Contains(err.Error(), "rate limit") {
		t.Errorf("want the rate-limit refusal named, got %v", err)
	}
	// the ref form is checked before any request goes out
	if _, err := resolveOne(t, root, "prs:opencharly/plugin-generate-podcast?limit=5", d); err == nil {
		t.Error("expected an error for a prs ref without merged-since")
	}
	if _, err := resolveOne(t, root, "issues:notarepo?state=all", d); err == nil {
		t.Error("expected an error for a ref that is not <org>/<repo>")
	}
}

func TestResolveChangelogWindowSpecAndRefusals(t *testing.T) {
	root := tree(t, "2026.274")
	b, err := resolveOne(t, root, "changelog-window:opencharly@2026.274", Deps{Tags: map[string][]string{"alpha": {"v2026.281.0829"}}})
	if err != nil {
		t.Fatalf("changelog-window through the resolver grammar: %v", err)
	}
	if len(b.Entries) != 3 || b.Cutoff != "2026.274" {
		t.Fatalf("want the 3 entries at/after the floor and the floor recorded, got %d entries, cutoff %q", len(b.Entries), b.Cutoff)
	}
	if len(b.Tag_gaps) != 1 || b.Tag_gaps[0].Tag != "v2026.281.0829" {
		t.Errorf("the tag reconciliation is lost when the resolver is driven by spec: %+v", b.Tag_gaps)
	}
	// the changelog-window spec keeps the REAL scan counts -- it walked the three repo dirs
	if b.Repos_scanned != 3 || b.Repos_with_entries != 3 {
		t.Errorf("the changelog-window scan counts were lost: scanned=%d with_entries=%d", b.Repos_scanned, b.Repos_with_entries)
	}

	// a word outside the vocabulary, a spec that resolves nothing, and colliding ids are all refused
	if _, err := resolveOne(t, root, "telepathy:alpha", Deps{}); err == nil {
		t.Error("expected an error for an unknown resolver word")
	}
	if _, err := resolveOne(t, root, "changelog-window:2099.001", Deps{}); err == nil {
		t.Error("expected an error when a spec resolves nothing")
	}
	if _, err := ParseSpec("file"); err == nil {
		t.Error("expected an error for a spec with no ref")
	}
	specA, _ := ParseSpec("file:alpha@HEAD:src/a.go")
	specB, _ := ParseSpec("glob:alpha@HEAD:src/a.go")
	if _, err := Resolve([]Spec{specA, specB}, Deps{Root: root}); err == nil {
		t.Error("expected an error when two specs produce the same source id")
	}
	// ... and a bundle assembled from explicit sources counts the repos it reaches rather than claiming
	// to have scanned a tree it never walked
	explicit, err := resolveOne(t, root, "file:alpha@HEAD:CHANGELOG/2026.280.0704.md", Deps{})
	if err != nil {
		t.Fatalf("file resolver: %v", err)
	}
	if explicit.Repos_scanned != 1 || explicit.Repos_with_entries != 1 {
		t.Errorf("explicit-source counts should be the repos the bundle reaches: %+v", explicit)
	}
	if _, err := time.Parse(time.RFC3339, b.Generated_at); err != nil {
		t.Errorf("generated_at is not RFC3339: %v", err)
	}
	if _, err := Resolve(nil, Deps{Root: root}); err == nil {
		t.Error("expected the source-free refusal")
	}
}
