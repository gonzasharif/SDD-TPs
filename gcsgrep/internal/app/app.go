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

// Run executes one gcsgrep run over argv (os.Args[1:]) and returns the
// process exit code (FR-8).
func Run(ctx context.Context, argv []string, stdout, stderr io.Writer, newClient ClientFactory) int {
	w := output.New(stdout, stderr)

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

	cfg := scanner.Config{
		Bucket:     args.Bucket,
		Prefix:     args.Prefix,
		MaxObjects: args.MaxObjects,
	}
	return scanner.Run(ctx, client, cfg, m, w)
}
