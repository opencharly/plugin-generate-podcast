package generatepodcast

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast/params"
)

const usage = `generate-podcast -- the podcast desk

  generate-podcast emit    --episode <validated.json> [--enthusiast NAME] [--anchor NAME]
  generate-podcast resolve --root <umbrella> --cutoff <YYYY.DDD>

emit turns a VALIDATED #Episode (cue export of schema/episode.cue) into the episode markdown and
the cue lines podcast-render consumes. resolve builds a #SourceBundle from the CHANGELOG entries at
or after the cutoff, with each entry's digest and the tag gaps reconciled.
`

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
	default:
		fmt.Fprintf(errw, "generate-podcast: unknown verb %q\n\n%s", args[0], usage)
		return 2
	}
}

func cliEmit(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(errw)
	episode := fs.String("episode", "", "path to the validated #Episode as JSON (cue export)")
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
	root := fs.String("root", "", "umbrella root to scan")
	cutoff := fs.String("cutoff", "", "CalVer floor, e.g. 2026.274")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *cutoff == "" {
		fmt.Fprintln(errw, "generate-podcast resolve: --root and --cutoff are required")
		return 2
	}
	b, err := ResolveChangelogWindow(*root, *cutoff, nil)
	if err != nil {
		fmt.Fprintf(errw, "generate-podcast resolve: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		fmt.Fprintf(errw, "generate-podcast resolve: %v\n", err)
		return 1
	}
	return 0
}

// CliMain is the sdk.Main entrypoint: Run against the process streams.
func CliMain(args []string) int { return Run(args, os.Stdout, os.Stderr) }
