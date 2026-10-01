package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/gcsclient/gcsclienttest"
)

type outcome struct {
	code           int
	stdout, stderr string
	factoryCalls   int
	gcsCalls       int
}

// runWith runs gcsgrep against fake, counting how many times the client
// factory was called and how many GCS calls the fake received.
func runWith(fake *gcsclienttest.Fake, factoryErr error, argv ...string) outcome {
	var stdout, stderr bytes.Buffer
	factoryCalls := 0
	factory := func(context.Context) (gcsclient.Client, error) {
		factoryCalls++
		if factoryErr != nil {
			return nil, factoryErr
		}
		return fake, nil
	}
	code := Run(context.Background(), argv, &stdout, &stderr, factory)
	return outcome{code, stdout.String(), stderr.String(), factoryCalls, fake.Calls()}
}

func logsData() *gcsclienttest.Fake {
	return &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "logs/a.log", Content: "INFO start\nERROR timeout\n"},
		{Name: "logs/b.log", Content: "INFO ok\n"},
	}}
}

// VC-1.4, VC-25.1, VC-31.1–31.4: invalid invocations print the literal
// mensaje de error, exit 2, and never create a GCS client nor call GCS.
func TestRun_InvalidInvocationsNeverTouchGCS(t *testing.T) {
	cases := []struct {
		name       string
		argv       []string
		wantStderr string // exact, unless wantPrefix
		wantPrefix bool
	}{
		{"VC-1.4 invalid pattern", []string{"(", "gs://b/logs/"}, `gcsgrep: error: invalid pattern "(": `, true},
		{"VC-25.1 location without gs://", []string{"timeout", "mybucket/logs/"}, "gcsgrep: error: invalid location \"mybucket/logs/\": must start with gs://\n", false},
		{"VC-31.1 unknown flag", []string{"-v", "timeout", "gs://b/logs/"}, "gcsgrep: error: unknown flag -v\n", false},
		{"VC-31.2 one argument", []string{"timeout"}, "gcsgrep: error: expected 2 arguments (PATTERN and LOCATION), got 1\n", false},
		{"VC-31.3 non-integer --max", []string{"--max", "diez", "timeout", "gs://b/logs/"}, "gcsgrep: error: invalid value \"diez\" for --max: must be an integer\n", false},
		{"VC-31.4 negative --max", []string{"--max", "-5", "timeout", "gs://b/logs/"}, "gcsgrep: error: invalid value \"-5\" for --max: must be >= 0\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runWith(logsData(), nil, tc.argv...)
			if got.code != 2 {
				t.Errorf("exit code = %d, want 2", got.code)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}
			if tc.wantPrefix {
				if !strings.HasPrefix(got.stderr, tc.wantStderr) || strings.Count(got.stderr, "\n") != 1 {
					t.Errorf("stderr = %q, want one line starting with %q", got.stderr, tc.wantStderr)
				}
			} else if got.stderr != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", got.stderr, tc.wantStderr)
			}
			if got.factoryCalls != 0 || got.gcsCalls != 0 {
				t.Errorf("factory calls = %d, GCS calls = %d; want 0 and 0", got.factoryCalls, got.gcsCalls)
			}
		})
	}
}

// VC-25.2 (unit-level slice) / FR-16.2: no ADC → literal mensaje de error,
// exit 2.
func TestRun_NoCredentials(t *testing.T) {
	got := runWith(logsData(), errors.New("credentials: could not find default credentials"), "timeout", "gs://b/logs/")
	if got.code != 2 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 2 and empty", got.code, got.stdout)
	}
	want := "gcsgrep: error: no Application Default Credentials found: credentials: could not find default credentials\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
}

// VC-1.1 end to end with the fake client.
func TestRun_Search(t *testing.T) {
	got := runWith(logsData(), nil, "timeout", "gs://b/logs/")
	if got.code != 0 || got.stdout != "logs/a.log:2:ERROR timeout\n" || got.stderr != "" {
		t.Errorf("got %+v", got)
	}
}

// VC-2.1 / FR-2: gs://bucket/ reaches the scanner as an empty prefix.
func TestRun_WholeBucketLocation(t *testing.T) {
	fake := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "a/1.log", Content: "timeout\n"},
		{Name: "raiz.log", Content: "timeout\n"},
	}}
	got := runWith(fake, nil, "timeout", "gs://b/")
	if got.code != 0 || got.stdout != "a/1.log:1:timeout\nraiz.log:1:timeout\n" {
		t.Errorf("got %+v", got)
	}
	if p := fake.ListPrefixes(); len(p) != 1 || p[0] != "" {
		t.Errorf("List prefixes = %q, want [\"\"]", p)
	}
}

// VC-29 / FR-20: -n produces byte-identical stdout, stderr and exit code.
func TestRun_NFlagHasNoEffect(t *testing.T) {
	without := runWith(logsData(), nil, "timeout", "gs://b/logs/")
	with := runWith(logsData(), nil, "-n", "timeout", "gs://b/logs/")
	if with.code != without.code || with.stdout != without.stdout || with.stderr != without.stderr {
		t.Errorf("-n changed the result: %+v vs %+v", with, without)
	}
}

// VC-4.1, VC-4.2 / FR-4: -i matches regardless of case; without it, it
// doesn't.
func TestRun_IgnoreCase(t *testing.T) {
	data := func() *gcsclienttest.Fake {
		return &gcsclienttest.Fake{Objects: []gcsclienttest.Object{{Name: "case/a.log", Content: "TIMEOUT error\n"}}}
	}
	with := runWith(data(), nil, "-i", "timeout", "gs://b/case/")
	if with.code != 0 || with.stdout != "case/a.log:1:TIMEOUT error\n" {
		t.Errorf("with -i: %+v", with)
	}
	without := runWith(data(), nil, "timeout", "gs://b/case/")
	if without.code != 1 || without.stdout != "" {
		t.Errorf("without -i: %+v", without)
	}
}

// VC-1.3 / FR-1.3: the pattern is RE2; an escaped metacharacter is literal.
func TestRun_PatternIsRE2(t *testing.T) {
	fake := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{{Name: "v/a.log", Content: "version 1.2\nversion 1x2\n"}}}
	got := runWith(fake, nil, `version 1\.[0-9]`, "gs://b/v/")
	if got.code != 0 || got.stdout != "v/a.log:1:version 1.2\n" {
		t.Errorf("got %+v", got)
	}
}
