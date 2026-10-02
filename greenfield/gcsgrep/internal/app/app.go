// Package app wires gcsgrep's modules together: cli → match → gcsclient →
// scanner. It lives outside package main so the whole invocation path can be
// tested with a fake GCS client, including the guarantee that invalid
// invocations never touch GCS (FR-1.4, FR-16.1, FR-22).
package app

import (
	"context"
	"io"

	"gcsgrep/internal/cli"
	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
	"gcsgrep/internal/scanner"
)

// ClientFactory builds the GCS client. It is only called once the invocation
// has been fully validated; production passes gcsclient.New.
type ClientFactory func(ctx context.Context) (gcsclient.Client, error)

// Terminals says which of the output streams are terminals (TTY).
type Terminals struct {
	Stdout bool
	Stderr bool
}

// Run is RunWithTerminals for streams that are not terminals.
func Run(ctx context.Context, argv []string, stdout, stderr io.Writer, newClient ClientFactory) int {
	return RunWithTerminals(ctx, argv, stdout, stderr, Terminals{}, newClient)
}

// RunWithTerminals executes one gcsgrep run over argv (os.Args[1:]) and
// returns the process exit code (FR-8). Color depends on stdout being a
// terminal (FR-3.2, FR-3.3) and the progress style on stderr being one
// (FR-10.1, FR-10.2).
func RunWithTerminals(ctx context.Context, argv []string, stdout, stderr io.Writer, term Terminals, newClient ClientFactory) int {
	w := output.NewWithOptions(stdout, stderr, output.Options{
		Color:          term.Stdout,
		Progress:       true,
		StderrTerminal: term.Stderr,
	})

	args, err := cli.Parse(argv)
	if err != nil {
		w.Error("%v", err) // FR-16.1, FR-22
		return scanner.ExitError
	}

	m, err := match.New(args.Pattern, args.IgnoreCase)
	if err != nil {
		w.Error("invalid pattern %q: %v", args.Pattern, err) // FR-1.4
		return scanner.ExitError
	}

	client, err := newClient(ctx)
	if err != nil {
		w.Error("no Application Default Credentials found: %v", err) // FR-16.2
		return scanner.ExitError
	}

	client = gcsclient.WithRetries(client, gcsclient.DefaultRetryPolicy) // NFR-3

	cfg := scanner.Config{
		Bucket:     args.Bucket,
		Prefix:     args.Prefix,
		MaxObjects: args.MaxObjects,

		MaxObjectSize: args.MaxObjectSize,
		MaxTotalSize:  args.MaxTotalSize,
		Mode:          args.Mode,
	}
	return scanner.Run(ctx, client, cfg, m, w)
}
