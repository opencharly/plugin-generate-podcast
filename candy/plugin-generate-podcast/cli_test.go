package generatepodcast

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
