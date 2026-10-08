package generatepodcast

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

const usage = `generate-podcast -- the podcast desk

  generate-podcast emit     --episode <validated.json> [--bundle <bundle.json>] [--enthusiast NAME] [--anchor NAME] [--cues]
  generate-podcast resolve  --root <checkout> [--cutoff <YYYY.DDD>] [--source <resolver>:<ref>]...
  generate-podcast validate --outputs <typed-outputs.json> --bundle <bundle.json> [--selection <selection.json>]

emit turns a VALIDATED #Episode (cue export of schema/episode.cue) into the episode markdown and the
cue lines podcast-render consumes. With --bundle it also runs the renderer's input gate: every segment's
receipt_source must name a source in that bundle, and the episode's source count must match it.

resolve builds a #SourceBundle. With no --source it walks every repo's CHANGELOG/ at or after the CalVer
floor (the changelog-window resolver, with each entry's digest and the tag gaps reconciled). With --source
it resolves the named refs instead; the resolver words are the schema's #Resolver vocabulary:

  file             repo@ref:path                  one document
  glob             repo@ref:src/**/*.go           a file set, each file a source
  changelog-window org@<calver-floor>             every repo's CHANGELOG/ at/after the floor
  git-log          repo@<range>:path              commits and their messages
  issues           org/repo?state=&label=&q=&limit=       a GitHub issue query
  prs              org/repo?merged-since=YYYY-MM-DD&label=&path=&limit=   a merged-PR query, body + diffstat
  doc              file://PATH | https://URL | pdf:PATH | PATH            extracted text
  doctrine         profile:org-voice | profile:org-rules                  a named file set
  episode          news-podcast:episodes/<slug>.md                        a prior episode

validate gates the writing stage's #TypedOutputs against the bundle: every claim's quote must appear
VERBATIM in the source it cites (the anti-fabrication gate), and coverage/gaps must match what was read.
`

// stringsFlag collects a repeatable flag (`--source a --source b`).
type stringsFlag []string

func (s *stringsFlag) String() string { return strings.Join(*s, ",") }

func (s *stringsFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// Run is the command face, with explicit streams so it is testable.
func Run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errw, usage)
		return 2
	}
	switch args[0] {
	case "emit":
		return cliEmit(args[1:], out, errw)
	case "resolve":
		return cliResolve(args[1:], out, errw)
	case "validate":
		return cliValidate(args[1:], out, errw)
	default:
		fmt.Fprintf(errw, "generate-podcast: unknown verb %q\n\n%s", args[0], usage)
		return 2
	}
}

func cliEmit(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(errw)
	episode := fs.String("episode", "", "path to the validated #Episode as JSON (cue export)")
	bundlePath := fs.String("bundle", "", "path to the #SourceBundle the episode was written against (enables the renderer's input gate)")
	enthusiast := fs.String("enthusiast", "", "speaker NAME for the enthusiast role")
	anchor := fs.String("anchor", "", "speaker NAME for the anchor role")
	cues := fs.Bool("cues", false, "print only the renderer's cue lines")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *episode == "" {
		fmt.Fprintln(errw, "generate-podcast emit: --episode is required")
		return 2
	}
	raw, err := os.ReadFile(*episode)
	if err != nil {
		fmt.Fprintf(errw, "generate-podcast emit: %v\n", err)
		return 1
	}
	var ep params.Episode
	if err := json.Unmarshal(raw, &ep); err != nil {
		// the input is validated against #Episode upstream; a decode failure here means it was not
		fmt.Fprintf(errw, "generate-podcast emit: %v (is this cue export output?)\n", err)
		return 1
	}
	if *bundlePath != "" {
		var b params.SourceBundle
		braw, err := os.ReadFile(*bundlePath)
		if err != nil {
			fmt.Fprintf(errw, "generate-podcast emit: %v\n", err)
			return 1
		}
		if err := json.Unmarshal(braw, &b); err != nil {
			fmt.Fprintf(errw, "generate-podcast emit: %v (is this cue export output?)\n", err)
			return 1
		}
		if err := ValidateEpisodeInputs(ep, b); err != nil {
			fmt.Fprintf(errw, "generate-podcast emit: %v\n", err)
			return 1
		}
	}
	md, cueList, err := Emit(ep, Cast{Enthusiast: *enthusiast, Anchor: *anchor})
	if err != nil {
		fmt.Fprintf(errw, "generate-podcast emit: %v\n", err)
		return 1
	}
	if *cues {
		for _, c := range cueList {
			fmt.Fprintln(out, c.String())
		}
		return 0
	}
	fmt.Fprint(out, md)
	return 0
}

func cliResolve(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(errw)
	root := fs.String("root", "", "checkout to resolve refs against")
	cutoff := fs.String("cutoff", "", "CalVer floor, e.g. 2026.274 (the default changelog-window walk)")
	var sources stringsFlag
	fs.Var(&sources, "source", "a <resolver>:<ref> to resolve; repeatable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *root == "" {
		fmt.Fprintln(errw, "generate-podcast resolve: --root is required")
		return 2
	}
	specs := make([]Spec, 0, len(sources)+1)
	for _, raw := range sources {
		spec, err := ParseSpec(raw)
		if err != nil {
			fmt.Fprintf(errw, "generate-podcast resolve: %v\n", err)
			return 2
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		if *cutoff == "" {
			fmt.Fprintln(errw, "generate-podcast resolve: --cutoff (or at least one --source) is required")
			return 2
		}
		specs = append(specs, Spec{Resolver: "changelog-window", Ref: *cutoff})
	}
	b, err := Resolve(specs, Deps{Root: *root, Cutoff: *cutoff})
	if err != nil {
		fmt.Fprintf(errw, "generate-podcast resolve: %v\n", err)
		return 1
	}
	return writeJSON(out, errw, b, "resolve")
}

func cliValidate(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(errw)
	outputsPath := fs.String("outputs", "", "path to the writing stage's #TypedOutputs as JSON")
	bundlePath := fs.String("bundle", "", "path to the #SourceBundle the outputs were written against")
	selectionPath := fs.String("selection", "", "path to the selection the outputs were written from (optional, sharpens the coverage check)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *outputsPath == "" || *bundlePath == "" {
		fmt.Fprintln(errw, "generate-podcast validate: --outputs and --bundle are required")
		return 2
	}
	var outputs params.TypedOutputs
	if err := readJSON(*outputsPath, &outputs); err != nil {
		fmt.Fprintf(errw, "generate-podcast validate: %v\n", err)
		return 1
	}
	var bundle params.SourceBundle
	if err := readJSON(*bundlePath, &bundle); err != nil {
		fmt.Fprintf(errw, "generate-podcast validate: %v\n", err)
		return 1
	}
	var sel *Selection
	if *selectionPath != "" {
		var s Selection
		if err := readJSON(*selectionPath, &s); err != nil {
			fmt.Fprintf(errw, "generate-podcast validate: %v\n", err)
			return 1
		}
		sel = &s
	}
	if err := ValidateOutputs(outputs, bundle, sel); err != nil {
		fmt.Fprintf(errw, "generate-podcast validate: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "validate: the typed outputs carry their receipts")
	return 0
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w (is this cue export output?)", path, err)
	}
	return nil
}

func writeJSON(out, errw io.Writer, v any, verb string) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(errw, "generate-podcast %s: %v\n", verb, err)
		return 1
	}
	return 0
}

// CliMain is the sdk.Main entrypoint: Run against the process streams.
func CliMain(args []string) int { return Run(args, os.Stdout, os.Stderr) }
