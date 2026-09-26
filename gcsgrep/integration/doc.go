// Package integration holds the tests that verify gcsgrep's VCs against real
// GCS, using the data set of the "Datos de prueba" section of
// gcsgrep-spec.md (created with testdata/setup-testdata.sh).
//
// The tests only compile with the "integration" build tag, so `go test ./...`
// never touches the network. Run them with:
//
//	GCSGREP_TEST_BUCKET=<bucket> GCSGREP_TEST_CREDS=<dir> \
//	  go test -tags integration -v -count=1 ./integration/
//
// GCSGREP_TEST_CREDS is a directory with viewer.json, restringida.json and
// sin-rol.json (impersonated ADC files for <sa-viewer>, <sa-restringida> and
// <sa-sin-rol>); the VCs that need a missing file are skipped, not failed.
// The measurement VCs (VC-21, VC-22) also need GCSGREP_BENCH=1.
package integration
