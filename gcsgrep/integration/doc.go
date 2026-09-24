// Package integration holds gcsgrep's end-to-end checks against real GCS.
// They live behind the "integration" build tag, so a plain `go test ./...`
// never touches the network or costs anything. See integration_test.go.
package integration
