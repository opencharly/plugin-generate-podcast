package generatepodcast

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// resolvers.go — the resolver LAYER: one adapter per `#Resolver` word. `--source <resolver>:<ref>` names
// one adapter; the set is open, so adding a source kind is adding an adapter and never changing the
// generator. The words here and the vocabulary the schema publishes (`#Resolver` in schema/source.cue)
// are ONE closed set, kept equal by TestResolversMatchSchemaVocabulary: a word in the vocabulary with no
// adapter (a published claim nobody serves) fails the build rather than failing at run time.

// Spec is one `--source <resolver>:<ref>` request. The split is on the FIRST colon, because several ref
// forms carry their own colons: `doc:https://…`, `doctrine:profile:org-rules`,
// `episode:news-podcast:episodes/<slug>.md`.
type Spec struct {
	Resolver string
	Ref      string
}

func ParseSpec(s string) (Spec, error) {
	i := strings.Index(s, ":")
	if i <= 0 || i == len(s)-1 {
		return Spec{}, fmt.Errorf("source %q is not <resolver>:<ref>", s)
	}
	return Spec{Resolver: strings.TrimSpace(s[:i]), Ref: strings.TrimSpace(s[i+1:])}, nil
}

// Deps is everything the resolvers take from the world, injected so each adapter is testable and none
// reaches for a global. `Git` and `Get` are the two boundary crossings, and their DEFAULTS are the real
// thing (`gitRun`, `httpGet`): an adapter runs against the live service or fails with a named reason
// (R7a) and never invents an answer. Setting them is a CALLER's seam -- the unit tests point them at a
// local server or an injected result -- never a fallback the adapter chooses for itself.
type Deps struct {
	Root    string              // the project/umbrella checkout the refs resolve against
	Cutoff  string              // CalVer floor, recorded in the bundle
	Tags    map[string][]string // repo -> tags, for the changelog-window reconciliation
	Git     func(dir string, args ...string) ([]byte, error)
	Get     func(rawurl string) (body []byte, status int, err error)
	APIBase string // GitHub REST base; defaults to https://api.github.com
	Now     func() time.Time
}

func (d Deps) git(dir string, args ...string) ([]byte, error) {
	if d.Git != nil {
		return d.Git(dir, args...)
	}
	return gitRun(dir, args...)
}

func (d Deps) get(rawurl string) ([]byte, int, error) {
	if d.Get != nil {
		return d.Get(rawurl)
	}
	return httpGet(rawurl, d.apiBase())
}

func (d Deps) apiBase() string {
	if d.APIBase != "" {
		return strings.TrimSuffix(d.APIBase, "/")
	}
	return "https://api.github.com"
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// resolverFunc appends what one adapter resolved. It writes into the bundle because two resolvers carry
// bundle-level data the per-source list cannot: changelog-window reconciles tag gaps and counts the
// repos it actually scanned.
type resolverFunc func(s Spec, d Deps, b *params.SourceBundle) error

// resolvers is the closed adapter set. Keys are the `#Resolver` words; the ref form each expects is
// documented on its function.
var resolvers = map[string]resolverFunc{
	"changelog-window": resolveChangelogWindow,
	"file":             resolveFile,
	"glob":             resolveGlob,
	"git-log":          resolveGitLog,
	"issues":           resolveIssues,
	"prs":              resolvePRs,
	"doc":              resolveDoc,
	"doctrine":         resolveDoctrine,
	"episode":          resolveEpisode,
}

// ResolverNames returns every implemented resolver word, sorted, so a usage/error message and the
// schema-vocabulary test both read the table rather than a hand-kept list.
func ResolverNames() []string {
	names := make([]string, 0, len(resolvers))
	for n := range resolvers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Resolve runs every spec and returns ONE bundle. Two refusals live here rather than in the adapters:
// an unknown resolver word, and a spec that resolved NOTHING (a silent hole is what §5.4 exists for).
// A duplicate source id is refused too — ids are the citations a #Claim points at, so a collision would
// silently re-point a receipt.
func Resolve(specs []Spec, d Deps) (params.SourceBundle, error) {
	if len(specs) == 0 {
		return params.SourceBundle{}, errors.New("resolve: no sources given -- a source-free run is refused (§5.4)")
	}
	var b params.SourceBundle
	b.Cutoff = d.Cutoff
	b.Generated_at = d.now().UTC().Format(time.RFC3339)

	origin := map[string]string{} // source id -> the spec that produced it
	for _, s := range specs {
		fn, ok := resolvers[s.Resolver]
		if !ok {
			return params.SourceBundle{}, fmt.Errorf("resolve: unknown resolver %q (known: %s)",
				s.Resolver, strings.Join(ResolverNames(), ", "))
		}
		before := len(b.Entries)
		if err := fn(s, d, &b); err != nil {
			return params.SourceBundle{}, err
		}
		if len(b.Entries) == before {
			return params.SourceBundle{}, fmt.Errorf(
				"resolve: %s:%s resolved no sources -- a pattern that matches nothing is a silent hole, not an empty answer (§5.4)",
				s.Resolver, s.Ref)
		}
		for _, e := range b.Entries[before:] {
			who := s.Resolver + ":" + s.Ref
			if prev, dup := origin[e.Id]; dup {
				return params.SourceBundle{}, fmt.Errorf(
					"resolve: source id %q is produced by both %s and %s; ids ARE the citations, so a collision is refused",
					e.Id, prev, who)
			}
			origin[e.Id] = who
		}
	}
	sort.Slice(b.Entries, func(i, j int) bool { return b.Entries[i].Id < b.Entries[j].Id })
	// A bundle assembled from explicit sources scans no directory: the honest numbers are the repos the
	// sources actually come from. changelog-window sets the real scan counts itself.
	if b.Repos_scanned == 0 {
		repos := map[string]bool{}
		for _, e := range b.Entries {
			if e.Repo != "" {
				repos[e.Repo] = true
			}
		}
		b.Repos_scanned = int64(len(repos))
		b.Repos_with_entries = int64(len(repos))
	}
	return b, nil
}

// newSource is the ONE place a digest and a byte count are computed, so every adapter produces a
// comparable receipt (R3: `digest` is mandatory, and it is the same digest everywhere). It also sets a
// DEFAULT title (the body's first heading); an adapter that knows a better name -- a commit subject, an
// issue or PR title, the ref itself for a URL document -- overrides it on the returned value.
func newSource(r params.Resolver, id, ref, repo, p string, body []byte) params.Source {
	sum := sha256.Sum256(body)
	return params.Source{
		Id:       id,
		Resolver: r,
		Ref:      ref,
		Digest:   hex.EncodeToString(sum[:]),
		Bytes:    int64(len(body)),
		Repo:     repo,
		Path:     p,
		Title:    firstHeading(string(body)),
		Body:     string(body),
	}
}

// splitRef splits the `provenance:body` form (`repo@ref:path`) used by `file`, `glob`, `git-log` and
// `episode`. A ref with no colon is a bare path, returned whole.
//
// A URL ref is ALSO returned whole, and that guard is explicit (`isURLRef`) rather than a "does the
// left half contain `://`" test: that test fires only when `://` precedes the LAST colon, so a
// bare-scheme URL like `https://example.com/x` would split into ("https", "//example.com/x") — the
// scheme colon mistaken for the separator. No caller passes a URL here today (`resolveDoc` switches on
// the ref's prefix itself, so `doc:https://…` never reaches this function), and the guard is what keeps
// that true if one ever does; `TestSplitRefContract` pins all three shapes.
func splitRef(ref string) (prov, body string) {
	if isURLRef(ref) {
		return "", ref
	}
	if i := strings.LastIndex(ref, ":"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// isURLRef reports whether a ref names a URL rather than a `provenance:path` pair.
func isURLRef(ref string) bool {
	for _, scheme := range []string{"http://", "https://", "file://"} {
		if strings.HasPrefix(ref, scheme) {
			return true
		}
	}
	return false
}

// splitProvenance splits `repo@ref` into its two halves; either may be empty.
func splitProvenance(prov string) (repo, at string) {
	if i := strings.LastIndex(prov, "@"); i >= 0 {
		return prov[:i], prov[i+1:]
	}
	return prov, ""
}

// absPath resolves a ref path against the root, leaving an already-absolute path alone.
func absPath(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

// repoPath resolves a ref path against `<root>/<repo>` when the ref named a provenance repo, and against
// the root when it did not. `file` and `glob` share it, so a pattern reads the same in both.
func repoPath(root, repo, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if repo != "" {
		return filepath.Join(root, repo, p)
	}
	return filepath.Join(root, p)
}

// readSource reads a file that must be a usable receipt: present, regular, and non-empty (bytes > 0).
func readSource(what, full string) ([]byte, error) {
	st, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s: %s is a directory, not a source file", what, full)
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%s: %s is empty -- a source with no bytes is not a receipt (§5.4)", what, full)
	}
	return body, nil
}

// resolveChangelogWindow — ref `<org>@<calver-floor>` or a bare floor. The umbrella walk itself lives in
// resolve.go; this adapter carries it into the uniform `<resolver>:<ref>` grammar and records the real
// scan counts and the tag gaps the reconciliation found.
func resolveChangelogWindow(s Spec, d Deps, b *params.SourceBundle) error {
	_, floor := splitProvenance(s.Ref)
	if floor == "" {
		floor = s.Ref
	}
	win, err := ResolveChangelogWindow(d.Root, floor, d.Tags)
	if err != nil {
		return err
	}
	b.Entries = append(b.Entries, win.Entries...)
	b.Tag_gaps = append(b.Tag_gaps, win.Tag_gaps...)
	b.Repos_scanned = win.Repos_scanned
	b.Repos_with_entries = win.Repos_with_entries
	if b.Cutoff == "" {
		b.Cutoff = floor
	}
	return nil
}

// resolveFile — ref `repo@ref:path` (or a bare path). ONE document.
func resolveFile(s Spec, d Deps, b *params.SourceBundle) error {
	prov, p := splitRef(s.Ref)
	if p == "" {
		return fmt.Errorf("file: ref %q names no path (form: repo@ref:path)", s.Ref)
	}
	repo, _ := splitProvenance(prov)
	body, err := readSource("file", repoPath(d.Root, repo, p))
	if err != nil {
		return fmt.Errorf("file: %s: %w", s.Ref, err)
	}
	b.Entries = append(b.Entries, newSource("file", "file:"+s.Ref, s.Ref, repo, p, body))
	return nil
}

// resolveGlob — ref `repo@ref:<pattern>`. A file SET, each file one source. `**` crosses directories
// (Go's filepath.Glob cannot), and the pattern is matched against the slash-separated path RELATIVE to
// the repo directory, so a pattern reads the same from any root.
func resolveGlob(s Spec, d Deps, b *params.SourceBundle) error {
	prov, pattern := splitRef(s.Ref)
	if pattern == "" {
		return fmt.Errorf("glob: ref %q names no pattern (form: repo@ref:src/**/*.go)", s.Ref)
	}
	repo, _ := splitProvenance(prov)
	base := d.Root
	if repo != "" {
		base = filepath.Join(d.Root, repo)
	}
	re, err := globRegexp(filepath.ToSlash(pattern))
	if err != nil {
		return fmt.Errorf("glob: %s: %w", s.Ref, err)
	}
	var matches []string
	walkErr := filepath.WalkDir(base, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			if de.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(base, p)
		if relErr != nil {
			return relErr
		}
		if rel = filepath.ToSlash(rel); re.MatchString(rel) {
			matches = append(matches, rel)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("glob: %s: %w", s.Ref, walkErr)
	}
	if len(matches) == 0 {
		return fmt.Errorf("glob: %q matched no files under %s -- a pattern that matches nothing is a silent hole", pattern, base)
	}
	sort.Strings(matches)
	for _, rel := range matches {
		body, err := readSource("glob", filepath.Join(base, rel))
		if err != nil {
			return fmt.Errorf("glob: %s: %w", s.Ref, err)
		}
		b.Entries = append(b.Entries, newSource("glob", "glob:"+prov+":"+rel, s.Ref, repo, rel, body))
	}
	return nil
}

// globRegexp compiles the glob subset the resolver documents: `**` crosses separators (including zero
// segments, so `src/**/*.go` matches `src/a.go`), `*` and `?` stay inside one segment.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// resolveGitLog — ref `repo@<range>:path`. Commits and their messages; `path` is optional, `<range>`
// defaults to the tip commit. The ref's `@` half carries the range, so the digest of each commit is its
// own receipt.
func resolveGitLog(s Spec, d Deps, b *params.SourceBundle) error {
	prov, p := splitRef(s.Ref)
	repo, rng := splitProvenance(prov)
	if repo == "" {
		return fmt.Errorf("git-log: ref %q names no repo (form: repo@<range>:path)", s.Ref)
	}
	if rng == "" {
		rng = "HEAD"
	}
	dir := filepath.Join(d.Root, repo)
	args := []string{"log", "--no-color", "--pretty=format:%x1e%H%x1f%aI%x1f%s%x1f%b", "--name-only", rng}
	if p != "" {
		args = append(args, "--", p)
	}
	out, err := d.git(dir, args...)
	if err != nil {
		return fmt.Errorf("git-log: %s: %w", s.Ref, err)
	}
	records := strings.Split(strings.TrimPrefix(string(out), "\x1e"), "\x1e")
	n := 0
	for _, rec := range records {
		rec = strings.TrimRight(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		f := strings.SplitN(rec, "\x1f", 4)
		if len(f) < 3 {
			return fmt.Errorf("git-log: %s: unparseable git record %q", s.Ref, rec)
		}
		sha, date, subject := f[0], f[1], f[2]
		body := subject + "\n" + date + "\n"
		if len(f) == 4 && strings.TrimSpace(f[3]) != "" {
			body += "\n" + f[3]
		}
		short := sha
		if len(short) > 12 {
			short = short[:12]
		}
		id := "git-log:" + repo + "@" + short
		src := newSource("git-log", id, repo+"@"+sha, repo, p, []byte(body))
		src.Title = subject
		b.Entries = append(b.Entries, src)
		n++
	}
	if n == 0 {
		return fmt.Errorf("git-log: %s resolved no commits -- the range is empty", s.Ref)
	}
	return nil
}

// resolveIssues — ref `org/repo?state=&label=&q=&limit=`. A GitHub issue query. The query is answered by
// the LIVE API (never from memory); without `GITHUB_TOKEN` GitHub allows 60 requests/hour, and a refusal
// names that limit instead of pretending the query returned nothing.
func resolveIssues(s Spec, d Deps, b *params.SourceBundle) error {
	repo, q, err := parseRepoQuery(s.Ref)
	if err != nil {
		return fmt.Errorf("issues: %w", err)
	}
	limit := boundedLimit(q.Get("limit"), 20)
	var rawURL string
	if text := strings.TrimSpace(q.Get("q")); text != "" {
		v := url.Values{}
		v.Set("q", "repo:"+repo+" is:issue "+text)
		v.Set("per_page", strconv.Itoa(limit))
		rawURL = d.apiBase() + "/search/issues?" + v.Encode()
	} else {
		v := url.Values{}
		state := q.Get("state")
		if state == "" {
			state = "all"
		}
		v.Set("state", state)
		if label := q.Get("label"); label != "" {
			v.Set("labels", label)
		}
		v.Set("per_page", strconv.Itoa(limit))
		rawURL = d.apiBase() + "/repos/" + repo + "/issues?" + v.Encode()
	}
	items, err := fetchItems(d, "issues", s.Ref, rawURL)
	if err != nil {
		return err
	}
	n := 0
	for _, it := range items {
		if it.Pull_request != nil {
			continue // the issues endpoint also returns PRs
		}
		if strings.TrimSpace(it.Title+it.Body) == "" {
			continue // an issue with neither a title nor a body carries nothing quotable
		}
		body := fmt.Sprintf("# %s [#%d]\n\n%s\n", it.Title, it.Number, it.Body)
		src := newSource("issues", "issues:"+repo+"#"+strconv.Itoa(it.Number), "issues:"+s.Ref, repo, "", []byte(body))
		src.Title = it.Title
		b.Entries = append(b.Entries, src)
		n++
	}
	if n == 0 {
		return fmt.Errorf("issues: %s matched no issues", s.Ref)
	}
	return nil
}

// resolvePRs — ref `org/repo?merged-since=<YYYY-MM-DD>&label=&path=&limit=`. A PR query: the merged PRs
// since the date, each with its body and its diffstat (the plan's "body + diffstat"), and optional
// filtering by a path the PR touched. Every extra filter costs one more live request per PR, so `limit`
// bounds the walk.
func resolvePRs(s Spec, d Deps, b *params.SourceBundle) error {
	repo, q, err := parseRepoQuery(s.Ref)
	if err != nil {
		return fmt.Errorf("prs: %w", err)
	}
	since := strings.TrimSpace(q.Get("merged-since"))
	if len(since) != len("2006-01-02") {
		return fmt.Errorf("prs: ref %q needs merged-since=<YYYY-MM-DD> (the date the window opens)", s.Ref)
	}
	limit := boundedLimit(q.Get("limit"), 5)
	wantPath := strings.TrimSpace(q.Get("path"))

	v := url.Values{}
	v.Set("state", "closed")
	v.Set("sort", "updated")
	v.Set("direction", "desc")
	v.Set("per_page", strconv.Itoa(limit))
	list, err := fetchItems(d, "prs", s.Ref, d.apiBase()+"/repos/"+repo+"/pulls?"+v.Encode())
	if err != nil {
		return err
	}
	n := 0
	for _, it := range list {
		if len(it.Merged_at) < 10 || it.Merged_at[:10] < since {
			continue
		}
		detail, err := fetchItem(d, "prs", s.Ref, d.apiBase()+"/repos/"+repo+"/pulls/"+strconv.Itoa(it.Number))
		if err != nil {
			return err
		}
		if wantPath != "" {
			files, err := fetchItems(d, "prs", s.Ref, d.apiBase()+"/repos/"+repo+"/pulls/"+strconv.Itoa(it.Number)+"/files?per_page=100")
			if err != nil {
				return err
			}
			touched := false
			for _, f := range files {
				if f.Filename == wantPath {
					touched = true
					break
				}
			}
			if !touched {
				continue
			}
		}
		// the reachable emptiness check is on the DATA: the Sprintf below always emits literal text, so a
		// post-format `body == ""` arm could never fire (B14(a) on #6, round 1)
		if strings.TrimSpace(it.Title+detail.Body) == "" {
			continue
		}
		body := fmt.Sprintf("# %s [#%d]\n\n%s\n\n+%d/-%d across %d file(s), merged %s (%s)\n",
			it.Title, it.Number, detail.Body, detail.Additions, detail.Deletions, detail.Changed_files,
			detail.Merged_at, detail.Html_url)
		src := newSource("prs", "prs:"+repo+"#"+strconv.Itoa(it.Number), "prs:"+s.Ref, repo, "", []byte(body))
		src.Title = it.Title
		b.Entries = append(b.Entries, src)
		n++
	}
	if n == 0 {
		return fmt.Errorf("prs: %s matched no merged PRs (the window may simply be empty -- a resolver that resolves nothing is refused, so widen it)", s.Ref)
	}
	return nil
}

// resolveDoc — ref `file://<path>`, `https://<url>`, `pdf:<path>` or a bare path. Extracted text, one
// document. A PDF is extracted with `pdftotext` when it is on PATH and refused with a named reason when
// it is not — never silently treated as text. Plaintext `http://` is REFUSED rather than fetched: an
// operator-chosen URL over an unauthenticated transport is not a document fetch worth having.
func resolveDoc(s Spec, d Deps, b *params.SourceBundle) error {
	ref := s.Ref
	var (
		body []byte
		err  error
		pth  string
	)
	switch {
	case strings.HasPrefix(ref, "file://"):
		pth = strings.TrimPrefix(ref, "file://")
		body, err = readSource("doc", pth)
	case strings.HasPrefix(ref, "http://"):
		err = errors.New("doc: plaintext http:// is refused -- use https:// (or file://)")
	case strings.HasPrefix(ref, "https://"):
		var status int
		body, status, err = d.get(ref)
		switch {
		case err != nil:
			err = fmt.Errorf("doc: %s: %w (the resolver never answers from memory)", ref, err)
		case status == 403 || status == 429:
			err = fmt.Errorf("doc: %s: the server refused the fetch (HTTP %d)", ref, status)
		case status != 200:
			err = fmt.Errorf("doc: %s: HTTP %d", ref, status)
		case bytes.ContainsRune(body, 0):
			err = fmt.Errorf("doc: %s returned binary content, not text", ref)
		case len(bytes.TrimSpace(body)) == 0:
			err = fmt.Errorf("doc: %s returned an empty document", ref)
		}
	case strings.HasPrefix(ref, "pdf:"):
		pth = strings.TrimPrefix(ref, "pdf:")
		body, err = pdfText(pth)
	default:
		pth = ref
		body, err = readSource("doc", absPath(d.Root, ref))
	}
	if err != nil {
		return err
	}
	src := newSource("doc", "doc:"+ref, ref, "", pth, body)
	if src.Title == "" {
		src.Title = ref
	}
	b.Entries = append(b.Entries, src)
	return nil
}

// pdfText shells out to `pdftotext` (poppler) and names the missing prerequisite rather than degrading.
func pdfText(p string) ([]byte, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, fmt.Errorf("doc: pdf:%s needs pdftotext on PATH (poppler-utils); it is not installed", p)
	}
	cmd := exec.Command("pdftotext", "-layout", p, "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("doc: pdf:%s: pdftotext failed: %v: %s", p, err, strings.TrimSpace(stderr.String()))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("doc: pdf:%s extracted no text (a scan needs OCR)", p)
	}
	return out, nil
}

// doctrineProfiles is a CLOSED table: a named set is a promise about which documents it holds, so a
// missing member is an error rather than a quietly smaller set.
var doctrineProfiles = map[string][]string{
	"org-voice": {"SOUL.md", "VISION.md", "GRIEVANCES.md"},
	"org-rules": {"AGENTS.md", "charly/AGENTS.md"},
}

// resolveDoctrine — ref `profile:<name>`. A named file set: the org's voice, or its rulebook.
func resolveDoctrine(s Spec, d Deps, b *params.SourceBundle) error {
	name := strings.TrimPrefix(s.Ref, "profile:")
	if name == s.Ref || name == "" {
		return fmt.Errorf("doctrine: ref %q is not profile:<name>", s.Ref)
	}
	files, ok := doctrineProfiles[name]
	if !ok {
		known := make([]string, 0, len(doctrineProfiles))
		for n := range doctrineProfiles {
			known = append(known, "profile:"+n)
		}
		sort.Strings(known)
		return fmt.Errorf("doctrine: no profile %q -- the known sets are %s", name, strings.Join(known, ", "))
	}
	for _, rel := range files {
		body, err := readSource("doctrine", absPath(d.Root, rel))
		if err != nil {
			return fmt.Errorf("doctrine: profile:%s: %w (a named set is a promise about its members)", name, err)
		}
		b.Entries = append(b.Entries, newSource("doctrine", "doctrine:"+name+":"+rel, s.Ref, "", rel, body))
	}
	return nil
}

// resolveEpisode — ref `<checkout>:episodes/<slug>.md`. A prior episode, for a follow-up. The checkout
// is a directory under the root (the news-podcast content repo), and the slug is the file.
func resolveEpisode(s Spec, d Deps, b *params.SourceBundle) error {
	prov, p := splitRef(s.Ref)
	if prov == "" || p == "" {
		return fmt.Errorf("episode: ref %q is not <checkout>:episodes/<slug>.md", s.Ref)
	}
	body, err := readSource("episode", absPath(d.Root, filepath.Join(prov, p)))
	if err != nil {
		return fmt.Errorf("episode: %s: %w", s.Ref, err)
	}
	b.Entries = append(b.Entries, newSource("episode", "episode:"+prov+":"+p, s.Ref, prov, p, body))
	return nil
}

// parseRepoQuery splits `org/repo?k=v&…` into its repo and query.
func parseRepoQuery(ref string) (string, url.Values, error) {
	repo, raw := ref, ""
	if i := strings.Index(ref, "?"); i >= 0 {
		repo, raw = ref[:i], ref[i+1:]
	}
	if strings.Count(repo, "/") != 1 || strings.HasPrefix(repo, "/") {
		return "", nil, fmt.Errorf("ref %q needs <org>/<repo>?<query>", ref)
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "", nil, fmt.Errorf("ref %q: %w", ref, err)
	}
	return repo, q, nil
}

func boundedLimit(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 100 {
		return def
	}
	return n
}

// ghItem is the union of the REST item shapes the issues/prs resolvers read: one struct, because the
// fields they need never conflict.
type ghItem struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	Merged_at     string `json:"merged_at"`
	Additions     int    `json:"additions"`
	Deletions     int    `json:"deletions"`
	Changed_files int    `json:"changed_files"`
	Html_url      string `json:"html_url"`
	Filename      string `json:"filename"`
	Pull_request  *struct {
		Url string `json:"url"`
	} `json:"pull_request"`
}

// fetchItems answers a LIST endpoint (`/issues`, `/pulls`, `/search/issues`, `/pulls/<n>/files`). The
// search endpoint wraps its list in `items`, so both shapes are accepted.
func fetchItems(d Deps, what, ref, rawURL string) ([]ghItem, error) {
	body, status, err := d.get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w (no network? the resolver never answers from memory)", what, ref, err)
	}
	if err := ghStatus(what, ref, status); err != nil {
		return nil, err
	}
	var list []ghItem
	if err := json.Unmarshal(body, &list); err != nil {
		var wrapped struct {
			Items []ghItem `json:"items"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			return nil, fmt.Errorf("%s: %s: the API answer is not a JSON list: %v", what, ref, err)
		}
		list = wrapped.Items
	}
	return list, nil
}

// fetchItem answers one OBJECT endpoint (`/pulls/<n>`).
func fetchItem(d Deps, what, ref, rawURL string) (ghItem, error) {
	body, status, err := d.get(rawURL)
	if err != nil {
		return ghItem{}, fmt.Errorf("%s: %s: %w (no network? the resolver never answers from memory)", what, ref, err)
	}
	if err := ghStatus(what, ref, status); err != nil {
		return ghItem{}, err
	}
	var it ghItem
	if err := json.Unmarshal(body, &it); err != nil {
		return ghItem{}, fmt.Errorf("%s: %s: the API answer is not JSON: %v", what, ref, err)
	}
	return it, nil
}

// ghStatus turns the two refusals a live API can answer with into named reasons. The rate-limit one
// matters most: it is the difference between "no results" and "no permission to ask".
func ghStatus(what, ref string, status int) error {
	switch {
	case status == 200:
		return nil
	case status == 403, status == 429:
		return fmt.Errorf("%s: %s: GitHub refused the request (HTTP %d, rate limit) -- set GITHUB_TOKEN to raise the unauthenticated 60/hour limit",
			what, ref, status)
	case status == 404:
		return fmt.Errorf("%s: %s: no such repository, or no access to it (HTTP 404)", what, ref)
	case status == 401:
		return fmt.Errorf("%s: %s: the API rejected the credential (HTTP 401)", what, ref)
	default:
		return fmt.Errorf("%s: %s: HTTP %d", what, ref, status)
	}
}

// gitRun is the default git boundary.
func gitRun(dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errors.New("git is not on PATH -- the git-log resolver shells out to git; install it or use another resolver")
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// httpGet is the default HTTP boundary: a live fetch, carrying the operator's credential ONLY to the
// GitHub API base. SCOPING THE TOKEN MATTERS: `Deps.get` also serves `resolveDoc`'s arbitrary-URL
// branch, so an unscoped `Authorization` header would ship `GITHUB_TOKEN` to whatever host a
// `doc:https://…` ref named — exfiltration by a ref an operator (or a generated profile) can author.
// Measured defect (T3 block on PR #5): the first revision attached the credential unconditionally.
// It never fabricates a response.
func httpGet(rawurl, apiBase string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "charly-generate-podcast")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && sameHost(rawurl, apiBase) {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	// capped so a hostile or enormous URL cannot exhaust memory
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// sameHost reports whether two URLs address the SAME host — the credential-scoping predicate. An
// unparseable URL on either side answers false: a credential is never sent on a guess.
func sameHost(rawurl, apiBase string) bool {
	u, err := url.Parse(rawurl)
	if err != nil || u.Host == "" {
		return false
	}
	base, err := url.Parse(apiBase)
	if err != nil || base.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, base.Host)
}
