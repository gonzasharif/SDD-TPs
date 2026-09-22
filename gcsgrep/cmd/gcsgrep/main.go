// Command gcsgrep searches the content of text objects under a GCS
// bucket/prefix without downloading them first. See gcsgrep-spec.md for the
// full contract; this file only wires cli → scanner → gcsclient together.
package main

import (
	"context"
	"fmt"
	"os"

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

	cfg := scanner.Config{
		Bucket:     args.Bucket,
		Prefix:     args.Prefix,
		MaxObjects: args.MaxObjects,
		Mode:       outputMode(args),
	}

	return scanner.Run(ctx, client, cfg, m, w)
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
