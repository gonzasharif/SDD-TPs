// Command gcsgrep searches the content of text objects under a GCS
// bucket/prefix without downloading them first. See gcsgrep-spec.md for the
// full contract; this file only plugs the real GCS client into app.Run.
package main

import (
	"context"
	"os"

	"gcsgrep/internal/app"
	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/tty"
)

func main() {
	term := app.Terminals{Stdout: tty.IsTerminal(os.Stdout), Stderr: tty.IsTerminal(os.Stderr)}
	os.Exit(app.RunWithTerminals(context.Background(), os.Args[1:], os.Stdout, os.Stderr, term, gcsclient.New))
}
