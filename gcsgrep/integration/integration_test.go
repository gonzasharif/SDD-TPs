//go:build integration

// End-to-end verification of gcsgrep against a real GCS bucket: each test
// builds the real binary, runs it, and checks its exit code and output, the
// same way a person or a script would. They are the "test de integración"
// column of gcsgrep-spec.md's coverage table.
//
// Run them with Application Default Credentials configured and the test
// bucket named either in GCSGREP_TEST_BUCKET or in gcsgrep/testenv.local.md
// (gitignored, a line like "Bucket: gs://name/"):
//
//	go test -tags integration -v ./integration/
//
// The bucket must hold the fixtures described in gcsgrep-cobertura-vc.md:
// logs/ (app1.log, app2.log, app3.log, icon.png), perf/ and mem/small.log.
// Tests that read the bucket are skipped when no bucket is configured.
// Optional variables: GCSGREP_BENCH_PREFIX (enables the VC-21 benchmark),
// GCSGREP_BENCH_MIN (objects/s it must reach), GCSGREP_BENCH_FIRST_MAX
// (seconds to the first result), GCSGREP_REQUIRE_SPEEDUP=1 (VC-13 must be
// strictly faster with -j 8).
package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var binary string

// TestMain builds the binary once; every test runs that same executable.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gcsgrep-it-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := "gcsgrep"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary = filepath.Join(dir, name)

	build := exec.Command("go", "build", "-o", binary, "../cmd/gcsgrep")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building gcsgrep failed:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// result is one run of the binary. stdout and stderr are captured through
// pipes, so gcsgrep sees no terminal: plain output, progress as lines.
type result struct {
	stdout, stderr string
	code           int
	elapsed        time.Duration
}

func run(t *testing.T, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	start := time.Now()
	err := cmd.Run()
	res := result{stdout: stdout.String(), stderr: stderr.String(), elapsed: time.Since(start)}
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("could not run gcsgrep %v: %v", args, err)
		}
		res.code = exitErr.ExitCode()
	}
	return res
}

// bucket returns the test bucket, or skips the test if none is configured.
func bucket(t *testing.T) string {
	t.Helper()
	if b := strings.TrimSpace(os.Getenv("GCSGREP_TEST_BUCKET")); b != "" {
		return b
	}
	if data, err := os.ReadFile(filepath.Join("..", "testenv.local.md")); err == nil {
		if m := regexp.MustCompile(`gs://([a-z0-9][a-z0-9._-]*)`).FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	t.Skip("no test bucket: set GCSGREP_TEST_BUCKET or create gcsgrep/testenv.local.md")
	return ""
}

func expectExit(t *testing.T, r result, want int) {
	t.Helper()
	if r.code != want {
		t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, want, r.stdout, r.stderr)
	}
}

func expectContains(t *testing.T, what, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Errorf("%s should contain %q, got:\n%s", what, want, s)
	}
}

// sortedLines splits output into non-empty lines in a canonical order: with
// several workers the order of objects is not the listing order.
func sortedLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

// concurrencies is what the plan asks for: the earlier iterations' VCs must
// keep passing sequentially and with --concurrency 8.
var concurrencies = []string{"1", "8"}

func eachConcurrency(t *testing.T, fn func(t *testing.T, j string)) {
	t.Helper()
	for _, j := range concurrencies {
		t.Run("j="+j, func(t *testing.T) { fn(t, j) })
	}
}

// VC-1, VC-3 (plain branch), VC-11: a search finds the match, the output is
// plain object:line:text, and the binary is skipped with a notice.
func TestVC1_BasicSearchPlainOutputSkipsBinary(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		r := run(t, "-j", j, "timeout", "gs://"+b+"/logs/")
		expectExit(t, r, 0)
		expectContains(t, "stdout", r.stdout, "logs/app1.log:2:")
		expectContains(t, "stdout", r.stdout, "connection timeout after 30s")
		if strings.Contains(r.stdout, "\x1b") {
			t.Errorf("redirected stdout must have no ANSI escapes: %q", r.stdout)
		}
		if strings.Contains(r.stdout, "icon.png") {
			t.Errorf("the binary must never appear in the results: %q", r.stdout)
		}
		expectContains(t, "stderr", r.stderr, "logs/icon.png")
		expectContains(t, "stderr", r.stderr, "skipped")
	})
}

// VC-4: -i finds TIMEOUT for the pattern "timeout while"; without -i, nothing.
func TestVC4_IgnoreCase(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		without := run(t, "-j", j, "timeout while", "gs://"+b+"/logs/")
		expectExit(t, without, 1)

		with := run(t, "-j", j, "-i", "timeout while", "gs://"+b+"/logs/")
		expectExit(t, with, 0)
		expectContains(t, "stdout", with.stdout, "logs/app3.log:2:")
	})
}

// VC-8: exit 1 when nothing matches, exit 2 for a bucket that does not exist.
func TestVC8_ExitCodes(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		none := run(t, "-j", j, "patron_inexistente_xyz", "gs://"+b+"/logs/")
		expectExit(t, none, 1)
		if none.stdout != "" {
			t.Errorf("no results expected, got %q", none.stdout)
		}

		missing := run(t, "-j", j, "timeout", "gs://gcsgrep-no-such-bucket-xyz-000/")
		expectExit(t, missing, 2)
	})
}

// VC-17: the object-count guardrail aborts before reading any content.
func TestVC17_ObjectCountGuardrail(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		r := run(t, "-j", j, "--max", "1", "timeout", "gs://"+b+"/logs/")
		expectExit(t, r, 2)
		if r.stdout != "" {
			t.Errorf("no content may be read past the guardrail, got %q", r.stdout)
		}
		expectContains(t, "stderr", r.stderr, "exceeds the limit")
	})
}

// VC-5: -l prints only the names of objects with a match.
func TestVC5_ListOnly(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		r := run(t, "-j", j, "-l", "timeout", "gs://"+b+"/logs/")
		expectExit(t, r, 0)
		if got := sortedLines(r.stdout); !slices.Equal(got, []string{"logs/app1.log"}) {
			t.Errorf("-l output = %v, want only logs/app1.log", got)
		}
	})
}

// VC-6: -c prints a count per text object, zeros included, none for the binary.
func TestVC6_CountOnly(t *testing.T) {
	b := bucket(t)
	eachConcurrency(t, func(t *testing.T, j string) {
		r := run(t, "-j", j, "-c", "timeout", "gs://"+b+"/logs/")
		expectExit(t, r, 0)
		want := []string{"logs/app1.log:1", "logs/app2.log:0", "logs/app3.log:0"}
		if got := sortedLines(r.stdout); !slices.Equal(got, want) {
			t.Errorf("-c output = %v, want %v", got, want)
		}
	})
}

// VC-7: -l and -c together are a usage error.
func TestVC7_ListAndCountAreExclusive(t *testing.T) {
	b := bucket(t)
	r := run(t, "-l", "-c", "timeout", "gs://"+b+"/logs/")
	expectExit(t, r, 2)
	if r.stdout != "" {
		t.Errorf("nothing may be searched, got %q", r.stdout)
	}
}

// VC-14 / VC-20: --concurrency outside 1..32 is rejected as a usage error,
// before any request to GCS (so this needs no bucket at all).
func TestVC14_ConcurrencyOutOfRangeIsRejected(t *testing.T) {
	for _, value := range []string{"33", "100", "0", "-1"} {
		for _, flagName := range []string{"-j", "--concurrency"} {
			t.Run(flagName+"="+value, func(t *testing.T) {
				r := run(t, flagName, value, "timeout", "gs://any-bucket/")
				expectExit(t, r, 2)
				expectContains(t, "stderr", r.stderr, "--concurrency")
				if r.stdout != "" {
					t.Errorf("nothing may be searched, got %q", r.stdout)
				}
			})
		}
	}
}

// VC-13: with 8 workers the run reports exactly what the sequential one
// does. perf/ may hold no match at all (exit 1): -c still prints one line
// per object, so the two runs are comparable either way; what matters is
// that both end the same way and never in an error (exit 2). The timings
// are logged; set GCSGREP_REQUIRE_SPEEDUP=1 to also require -j 8 to be
// faster (it depends on network latency).
func TestVC13_ConcurrentMatchesSequential(t *testing.T) {
	b := bucket(t)
	seq := run(t, "-c", "-j", "1", "timeout", "gs://"+b+"/perf/")
	par := run(t, "-c", "-j", "8", "timeout", "gs://"+b+"/perf/")

	for name, r := range map[string]result{"sequential": seq, "-j 8": par} {
		if r.code != 0 && r.code != 1 {
			t.Fatalf("%s run failed with exit %d\nstderr:\n%s", name, r.code, r.stderr)
		}
	}
	if seq.code != par.code {
		t.Errorf("exit codes differ: sequential %d, -j 8 %d", seq.code, par.code)
	}
	got, want := sortedLines(par.stdout), sortedLines(seq.stdout)
	if len(want) == 0 {
		t.Fatalf("the sequential run found no objects under perf/: %s", seq.stderr)
	}
	if !slices.Equal(got, want) {
		t.Errorf("-j 8 results differ from sequential\nsequential: %v\nconcurrent: %v", want, got)
	}

	t.Logf("%d objects: sequential %.2fs (%.2f objects/s), -j 8 %.2fs (%.2f objects/s), %.2fx faster",
		len(want), seq.elapsed.Seconds(), float64(len(want))/seq.elapsed.Seconds(),
		par.elapsed.Seconds(), float64(len(want))/par.elapsed.Seconds(),
		seq.elapsed.Seconds()/par.elapsed.Seconds())
	if os.Getenv("GCSGREP_REQUIRE_SPEEDUP") == "1" && par.elapsed >= seq.elapsed {
		t.Errorf("-j 8 (%s) was not faster than sequential (%s)", par.elapsed, seq.elapsed)
	}
}

// VC-19 under concurrency: the run-wide limit is reported and the run exits
// 2. Repeated because a race in the shared budget only shows up sometimes.
func TestVC19_TotalSizeLimitUnderConcurrency(t *testing.T) {
	b := bucket(t)
	for i := 1; i <= 5; i++ {
		t.Run("run"+strconv.Itoa(i), func(t *testing.T) {
			r := run(t, "-j", "8", "--max-total-size", "10MiB", "-c", "timeout", "gs://"+b+"/")
			expectExit(t, r, 2)
			expectContains(t, "stderr", r.stderr, "total size limit of 10485760 bytes reached")
		})
	}
}

// VC-21: throughput and time to the first result with -j 8. It reads a
// lot, so it only runs when GCSGREP_BENCH_PREFIX names the prefix to scan
// (the spec asks for >= 500 objects of ~1 MiB). It uses -c so that every
// object prints exactly one line whether or not it matches: a line is a
// finished object, and the first one is the "first result". The thresholds
// are checked only if GCSGREP_BENCH_MIN / GCSGREP_BENCH_FIRST_MAX are set;
// otherwise the numbers are just logged.
func TestVC21_ConcurrentThroughput(t *testing.T) {
	prefix := os.Getenv("GCSGREP_BENCH_PREFIX")
	if prefix == "" {
		t.Skip("set GCSGREP_BENCH_PREFIX (e.g. perf500/) to run the benchmark")
	}
	b := bucket(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-j", "8", "--max", "0", "-c", "timeout", "gs://"+b+"/"+prefix)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var first time.Duration
	objects := 0
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		if objects == 0 {
			first = time.Since(start)
		}
		objects++
	}
	_ = cmd.Wait() // an exit 2 (a skipped object) still leaves a valid measurement
	total := time.Since(start)
	if objects == 0 {
		t.Fatal("the benchmark prefix produced no objects")
	}

	rate := float64(objects) / total.Seconds()
	t.Logf("%d objects in %.2fs = %.2f objects/s; first result after %.2fs", objects, total.Seconds(), rate, first.Seconds())

	if v := os.Getenv("GCSGREP_BENCH_MIN"); v != "" {
		min, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatalf("GCSGREP_BENCH_MIN: %v", err)
		}
		if rate < min {
			t.Errorf("throughput %.2f objects/s is below the threshold of %.2f", rate, min)
		}
	}
	if v := os.Getenv("GCSGREP_BENCH_FIRST_MAX"); v != "" {
		max, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatalf("GCSGREP_BENCH_FIRST_MAX: %v", err)
		}
		if first.Seconds() > max {
			t.Errorf("first result after %.2fs, above the limit of %.2fs", first.Seconds(), max)
		}
	}
}
