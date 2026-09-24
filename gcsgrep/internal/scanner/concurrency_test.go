package scanner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/output"
)

// parallelClient wraps a fakeClient and observes how many objects are open
// at once. Each stream sleeps briefly on its first read so that, with
// several workers, opens genuinely overlap.
type parallelClient struct {
	fakeClient
	inflight    atomic.Int32
	maxInflight atomic.Int32
	opens       atomic.Int32
}

func (p *parallelClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	rc, err := p.fakeClient.Open(ctx, bucket, object)
	if err != nil {
		return nil, err
	}
	p.opens.Add(1)
	now := p.inflight.Add(1)
	for {
		prev := p.maxInflight.Load()
		if now <= prev || p.maxInflight.CompareAndSwap(prev, now) {
			break
		}
	}
	return &slowStream{ReadCloser: rc, onClose: func() { p.inflight.Add(-1) }}, nil
}

type slowStream struct {
	io.ReadCloser
	once    sync.Once
	onClose func()
}

func (s *slowStream) Read(p []byte) (int, error) {
	s.once.Do(func() { time.Sleep(5 * time.Millisecond) })
	return s.ReadCloser.Read(p)
}

func (s *slowStream) Close() error {
	s.onClose()
	return s.ReadCloser.Close()
}

// manyObjects builds n objects named obj000.. in listing order; every
// third one contains "timeout".
func manyObjects(n int) (map[string]string, []string) {
	objects := make(map[string]string, n)
	names := make([]string, 0, n)
	for i := range n {
		name := fmt.Sprintf("logs/obj%03d.log", i)
		content := fmt.Sprintf("line one of %d\nnothing here\n", i)
		if i%3 == 0 {
			content += fmt.Sprintf("connection timeout in %d\n", i)
		}
		objects[name] = content
		names = append(names, name)
	}
	return objects, names
}

func sortedLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	sort.Strings(lines)
	return lines
}

func runWith(t *testing.T, client gcsclient.Client, cfg Config, pattern string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cfg.Bucket = "b"
	code = Run(context.Background(), client, cfg, mustMatcher(t, pattern), output.New(&out, &errOut))
	return code, out.String(), errOut.String()
}

// VC-13 (functional half): with 8 workers the run reports exactly the same
// results and exit code as the sequential one; only the order may differ.
func TestRun_ConcurrentResultsMatchSequential(t *testing.T) {
	objects, names := manyObjects(60)
	seqCode, seqOut, _ := runWith(t, &fakeClient{objects: objects, listedNames: names}, Config{MaxObjects: DefaultMaxObjects}, "timeout")
	conCode, conOut, _ := runWith(t, &fakeClient{objects: objects, listedNames: names}, Config{MaxObjects: DefaultMaxObjects, Concurrency: 8}, "timeout")

	if seqCode != ExitMatch || conCode != seqCode {
		t.Fatalf("exit codes: sequential %d, concurrent %d, want both %d", seqCode, conCode, ExitMatch)
	}
	seq, con := sortedLines(seqOut), sortedLines(conOut)
	if len(seq) != 20 {
		t.Fatalf("sequential run found %d matches, want 20", len(seq))
	}
	if strings.Join(seq, "\n") != strings.Join(con, "\n") {
		t.Errorf("concurrent results differ from sequential\nsequential: %v\nconcurrent: %v", seq, con)
	}
}

// Without the flag the run is strictly sequential: results come out in
// listing order, one object at a time (the documented default).
func TestRun_DefaultIsSequentialAndOrdered(t *testing.T) {
	objects, names := manyObjects(30)
	client := &parallelClient{fakeClient: fakeClient{objects: objects, listedNames: names}}

	_, out, _ := runWith(t, client, Config{MaxObjects: DefaultMaxObjects}, "timeout")

	if got := client.maxInflight.Load(); got != 1 {
		t.Errorf("max objects open at once = %d, want 1 in the default mode", got)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if !sort.StringsAreSorted(lines) {
		t.Errorf("sequential output should follow listing order: %v", lines)
	}
}

// FR-13: N workers really read N objects at the same time, and never more
// than N.
func TestRun_WorkersReadInParallelUpToTheLimit(t *testing.T) {
	objects, names := manyObjects(40)
	client := &parallelClient{fakeClient: fakeClient{objects: objects, listedNames: names}}

	code, _, _ := runWith(t, client, Config{MaxObjects: DefaultMaxObjects, Concurrency: 4}, "timeout")

	if code != ExitMatch {
		t.Fatalf("exit code = %d, want %d", code, ExitMatch)
	}
	if got := client.maxInflight.Load(); got < 2 || got > 4 {
		t.Errorf("max objects open at once = %d, want between 2 and 4 with 4 workers", got)
	}
	if got := client.opens.Load(); got != 40 {
		t.Errorf("opened %d objects, want each of the 40 exactly once", got)
	}
}

// Asking for more workers than objects is harmless.
func TestRun_MoreWorkersThanObjects(t *testing.T) {
	objects, names := manyObjects(3)
	client := &parallelClient{fakeClient: fakeClient{objects: objects, listedNames: names}}

	code, out, _ := runWith(t, client, Config{MaxObjects: DefaultMaxObjects, Concurrency: 32}, "timeout")

	if code != ExitMatch || len(sortedLines(out)) != 1 {
		t.Errorf("code = %d, out = %q, want exit 0 and the single match", code, out)
	}
	if got := client.maxInflight.Load(); got > 3 {
		t.Errorf("%d objects open at once with only 3 objects listed", got)
	}
}

// FR-9 / FR-8 with workers: an unreadable object is reported, the rest are
// still processed, and the run exits 2.
func TestRun_ConcurrentUnreadableObjectStillExitsError(t *testing.T) {
	objects, names := manyObjects(30)
	client := &fakeClient{objects: objects, listedNames: names, unreadable: map[string]bool{"logs/obj010.log": true}}

	code, out, errOut := runWith(t, client, Config{MaxObjects: DefaultMaxObjects, Concurrency: 8}, "timeout")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if got := len(sortedLines(out)); got != 10 {
		t.Errorf("got %d matches, want all 10 from the readable objects", got)
	}
	if !strings.Contains(errOut, "logs/obj010.log") {
		t.Errorf("stderr should name the unreadable object: %q", errOut)
	}
}

// BR-3 with workers: the count guardrail still aborts before any object is
// opened.
func TestRun_ConcurrentObjectCountGuardrailStillAborts(t *testing.T) {
	objects, names := manyObjects(10)
	client := &parallelClient{fakeClient: fakeClient{objects: objects, listedNames: names}}

	code, _, _ := runWith(t, client, Config{MaxObjects: 5, Concurrency: 8}, "timeout")

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if got := client.opens.Load(); got != 0 {
		t.Errorf("opened %d objects, want 0 after the guardrail", got)
	}
}

// BR-5 with workers (VC-19 under load): the workers together never read
// past --max-total-size, the run stops opening objects, reports it, and
// exits 2. Repeated because a race in the budget shows up only some of
// the time.
func TestRun_ConcurrentTotalSizeLimitIsNeverExceeded(t *testing.T) {
	const (
		objectCount = 40
		linesEach   = 8_000 // 8 bytes each: 64 KB per object
		limit       = 100_000
	)
	line := "timeout\n"
	content := strings.Repeat(line, linesEach)
	objects := make(map[string]string, objectCount)
	var names []string
	for i := range objectCount {
		name := fmt.Sprintf("big/obj%02d.log", i)
		objects[name] = content
		names = append(names, name)
	}

	for attempt := range 10 {
		client := &parallelClient{fakeClient: fakeClient{objects: objects, listedNames: names}}
		code, out, errOut := runWith(t, client, Config{MaxObjects: DefaultMaxObjects, Concurrency: 8, MaxTotalSize: limit}, "timeout")

		if code != ExitError {
			t.Fatalf("attempt %d: exit code = %d, want %d", attempt, code, ExitError)
		}
		// Every printed match is a full line that was read, so the printed
		// bytes are a lower bound on the bytes read: they must fit the limit.
		if printed := strings.Count(out, "timeout") * len(line); printed > limit {
			t.Fatalf("attempt %d: %d bytes of matching lines printed, more than the limit of %d", attempt, printed, limit)
		}
		if !strings.Contains(errOut, "total size limit") {
			t.Fatalf("attempt %d: stderr should report the limit: %q", attempt, errOut)
		}
		if got := client.opens.Load(); got >= objectCount {
			t.Fatalf("attempt %d: all %d objects were opened, the run should have stopped early", attempt, got)
		}
	}
}

// FR-10 with workers: every object advances the shared counter once, so a
// run of 20 ends at 20/20.
func TestRun_ConcurrentProgressCountsEveryObject(t *testing.T) {
	objects, names := manyObjects(20)
	client := &fakeClient{objects: objects, listedNames: names}
	var out, errOut bytes.Buffer
	w := output.New(&out, &errOut)
	w.Progress = output.ProgressLines

	Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, Concurrency: 8}, mustMatcher(t, "timeout"), w)

	if !strings.Contains(errOut.String(), "20/20 objects (100%)") {
		t.Errorf("progress should end at 20/20: %q", errOut.String())
	}
}
