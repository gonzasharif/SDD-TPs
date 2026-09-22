// Command gcsgrep searches the content of text objects under a GCS
// bucket/prefix without downloading them first. See gcsgrep-spec.md for the
// full contract; this file only wires cli → scanner → gcsclient together.
package main

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/term"

	"gcsgrep/internal/cli"
	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
	"gcsgrep/internal/reader"
	"gcsgrep/internal/scanner"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	w := output.New(os.Stdout, os.Stderr)
	w.Color = isTerminal(os.Stdout)
	w.Progress = output.ProgressLines
	if isTerminal(os.Stderr) {
		w.Progress = output.ProgressBar
	}

	args, err := cli.Parse(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcsgrep:", err)
		return scanner.ExitError
	}

	m, err := match.New(args.Pattern, args.IgnoreCase)
	if err != nil {
		w.Error("invalid pattern: %v", err)
		return scanner.ExitError
	}

	ctx := context.Background()
	client, err := gcsclient.New(ctx)
	if err != nil {
		w.Error("could not initialize the GCS client: %v", err)
		return scanner.ExitError
	}
	client = gcsclient.WithRetries(client)

	cfg := scanner.Config{
		Bucket:     args.Bucket,
		Prefix:     args.Prefix,
		MaxObjects: args.MaxObjects,
		Mode:       outputMode(args),

		MaxObjectSize: args.MaxObjectSize,
		MaxTotalSize:  args.MaxTotalSize,
		MaxLineSize:   int(args.MaxLineSize),
	}

	return scanner.Run(ctx, client, cfg, m, w)
}

// isTerminal is the isatty check behind FR-3's color and FR-10's progress
// style: true for a terminal, false for a file, a pipe, or /dev/null.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// outputMode maps the -l/-c flags to the reader mode. cli.Parse already
// guarantees at most one of them is set (FR-7).
func outputMode(args cli.Args) reader.Mode {
	switch {
	case args.ListOnly:
		return reader.ModeList
	case args.CountOnly:
		return reader.ModeCount
	default:
		return reader.ModeLines
	}
}
