package generatepodcast

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

// The declared `command:generate-podcast` word must be served by real code: this drives the CLI face
// end to end, from a validated-episode JSON on disk to the markdown and the renderer's cue lines.
func TestCliEmitFromJSON(t *testing.T) {
	raw, err := json.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "episode.json")
	if err := os.WriteFile(f, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	rc := Run([]string{"emit", "--episode", f, "--enthusiast", "CONCH", "--anchor", "TEMPER"}, &out, &errw)
	if rc != 0 {
		t.Fatalf("emit exited %d: %s", rc, errw.String())
	}
	for _, want := range []string{"coverage: {selected: 4, skipped: 592", "TEMPER: a verbatim passage [src-1]"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("markdown output is missing %q", want)
		}
	}

	out.Reset()
	rc = Run([]string{"emit", "--episode", f, "--enthusiast", "CONCH", "--anchor", "TEMPER", "--cues"}, &out, &errw)
	if rc != 0 {
		t.Fatalf("emit --cues exited %d: %s", rc, errw.String())
	}
	lines := strings.Fields(strings.TrimSpace(out.String()))
	if len(lines) == 0 || !strings.HasPrefix(out.String(), "CONCH:") {
		t.Errorf("cue output should start with the enthusiast's line, got %q", out.String()[:min(60, len(out.String()))])
	}
	if !strings.Contains(out.String(), "--") {
		t.Error("cue output should carry the between-segment pauses")
	}
}

// An unmapped role must be refused through the CLI too, not rendered as an unknown speaker.
func TestCliEmitRefusesUncastRole(t *testing.T) {
	raw, _ := json.Marshal(sample())
	f := filepath.Join(t.TempDir(), "episode.json")
	_ = os.WriteFile(f, raw, 0o644)
	var out, errw bytes.Buffer
	if rc := Run([]string{"emit", "--episode", f, "--enthusiast", "CONCH", "--anchor", "NOBODY"}, &out, &errw); rc == 0 {
		t.Fatal("expected a non-zero exit for an anchor that is not in the cast")
	}
}

// Usage errors exit 2 -- distinct from a real failure, so a caller can tell them apart.
func TestCliUsageErrors(t *testing.T) {
	var out, errw bytes.Buffer
	if rc := Run(nil, &out, &errw); rc != 2 {
		t.Errorf("no args should exit 2, got %d", rc)
	}
	errw.Reset()
	if rc := Run([]string{"emit"}, &out, &errw); rc != 2 {
		t.Errorf("emit without --episode should exit 2, got %d", rc)
	}
	errw.Reset()
	if rc := Run([]string{"nonsense"}, &out, &errw); rc != 2 {
		t.Errorf("an unknown verb should exit 2, got %d", rc)
	}
}

// resolve drives the resolver through the CLI against a real (temp) tree.
func TestCliResolve(t *testing.T) {
	root := tree(t, "2026.274")
	var out, errw bytes.Buffer
	if rc := Run([]string{"resolve", "--root", root, "--cutoff", "2026.274"}, &out, &errw); rc != 0 {
		t.Fatalf("resolve exited %d: %s", rc, errw.String())
	}
	var b map[string]any
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatalf("resolve output is not JSON: %v", err)
	}
	if n, _ := b["entries"].([]any); len(n) != 3 {
		t.Errorf("want 3 entries in the bundle, got %d", len(n))
	}
	errw.Reset()
	if rc := Run([]string{"resolve", "--root", root}, &out, &errw); rc != 2 {
		t.Errorf("resolve without --cutoff should exit 2, got %d", rc)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// The new leaves must be REACHABLE from the shipped command, not only from Go: `resolve --source` drives
// every adapter through the CLI grammar the skill documents.
func TestCliResolveWithSources(t *testing.T) {
	root := resolverRoot(t)
	var out, errw bytes.Buffer
	rc := Run([]string{"resolve", "--root", root,
		"--source", "file:alpha@HEAD:README.md",
		"--source", "glob:alpha@HEAD:src/**/*.go",
		"--source", "doc:notes.md",
		"--source", "doctrine:profile:org-rules",
		"--source", "episode:news-podcast:episodes/2026-10-01-prior.md",
	}, &out, &errw)
	if rc != 0 {
		t.Fatalf("resolve --source exited %d: %s", rc, errw.String())
	}
	var b params.SourceBundle
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatalf("resolve output is not a bundle: %v", err)
	}
	// sorted by id, which is what makes two runs over one ref set produce one bundle
	want := []string{
		"doc:notes.md", "doctrine:org-rules:AGENTS.md", "doctrine:org-rules:charly/AGENTS.md",
		"episode:news-podcast:episodes/2026-10-01-prior.md", "file:alpha@HEAD:README.md",
		"glob:alpha@HEAD:src/a.go", "glob:alpha@HEAD:src/deep/b.go",
	}
	if got := ids(b); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("bundle entries:\n got %v\nwant %v", got, want)
	}
	for _, e := range b.Entries {
		if len(e.Digest) != 64 || e.Bytes == 0 {
			t.Errorf("entry %s is not a receipt: %+v", e.Id, e)
		}
	}

	// a malformed spec is a usage error (2); an unknown resolver or an unresolvable ref is a failure (1)
	out.Reset()
	errw.Reset()
	if rc := Run([]string{"resolve", "--root", root, "--source", "file"}, &out, &errw); rc != 2 {
		t.Errorf("a spec without a ref should exit 2, got %d", rc)
	}
	errw.Reset()
	if rc := Run([]string{"resolve", "--root", root, "--source", "telepathy:alpha"}, &out, &errw); rc != 1 {
		t.Errorf("an unknown resolver should exit 1, got %d", rc)
	}
	errw.Reset()
	if rc := Run([]string{"resolve", "--root", root}, &out, &errw); rc != 2 {
		t.Errorf("resolve with neither --cutoff nor --source should exit 2, got %d", rc)
	}
}

// The validate leaf is the writing stage's gate, and it must FAIL on a fabricated quote.
func TestCliValidate(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, v any) string {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	b := receiptBundle()
	bundlePath := write("bundle.json", b)
	good := write("good.json", typedOutputs("it was silently ignoring fail"))
	bad := write("bad.json", typedOutputs("we made agents unstoppable"))

	var out, errw bytes.Buffer
	if rc := Run([]string{"validate", "--outputs", good, "--bundle", bundlePath}, &out, &errw); rc != 0 {
		t.Fatalf("validate exited %d on a verbatim quote: %s", rc, errw.String())
	}
	if !strings.Contains(out.String(), "receipts") {
		t.Errorf("validate should say what it proved, got %q", out.String())
	}
	out.Reset()
	if rc := Run([]string{"validate", "--outputs", bad, "--bundle", bundlePath}, &out, &errw); rc != 1 {
		t.Fatalf("validate accepted a fabricated quote (exit %d)", rc)
	}
	errw.Reset()
	if rc := Run([]string{"validate", "--outputs", good}, &out, &errw); rc != 2 {
		t.Errorf("validate without --bundle should exit 2, got %d", rc)
	}
}

// emit --bundle is the renderer's input gate: an episode citing a source outside the bundle never
// reaches the cue lines.
func TestCliEmitWithBundleGate(t *testing.T) {
	dir := t.TempDir()
	b := receiptBundle()
	raw, _ := json.Marshal(b)
	bundlePath := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundlePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	ep := sample()
	ep.Front_matter.Sources = int64(len(b.Entries))
	for i := range ep.Segments {
		ep.Segments[i].Receipt_source = "alpha@2026.280.0704"
	}
	epRaw, _ := json.Marshal(ep)
	epPath := filepath.Join(dir, "episode.json")
	if err := os.WriteFile(epPath, epRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	if rc := Run([]string{"emit", "--episode", epPath, "--bundle", bundlePath,
		"--enthusiast", "CONCH", "--anchor", "TEMPER", "--cues"}, &out, &errw); rc != 0 {
		t.Fatalf("emit --bundle exited %d for a resolvable episode: %s", rc, errw.String())
	}
	if !strings.Contains(out.String(), "alpha@2026.280.0704") {
		t.Errorf("the cue lines should carry the receipt's citation, got %q", out.String())
	}
	// the same episode against a bundle it does not cite
	other := params.SourceBundle{Entries: []params.Source{{Id: "x@2026.281.0001", Bytes: 1, Digest: "d"}}}
	oraw, _ := json.Marshal(other)
	opath := filepath.Join(dir, "other.json")
	if err := os.WriteFile(opath, oraw, 0o644); err != nil {
		t.Fatal(err)
	}
	errw.Reset()
	if rc := Run([]string{"emit", "--episode", epPath, "--bundle", opath,
		"--enthusiast", "CONCH", "--anchor", "TEMPER"}, &out, &errw); rc != 1 {
		t.Fatalf("the renderer's input gate let an unresolvable citation through (exit %d)", rc)
	}
}
