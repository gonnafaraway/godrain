package analyze

import (
	"testing"
	"time"

	"github.com/gonnafaraway/godrain/internal/allowlist"
	"github.com/gonnafaraway/godrain/internal/profile"
	"github.com/gonnafaraway/godrain/internal/sample"
)

func TestAnalyzeLinearGrowthLeak(t *testing.T) {
	points := syntheticSeries(t, 10, func(i int) int64 {
		return int64(i) * 2 * 1024 * 1024 // +2 MiB each sample
	}, "main.leakLoop")

	th := DefaultThresholds()
	th.Warmup = 0
	th.MinWindow = 20 * time.Second
	th.SuspectSlope = 10 * 1024
	th.LeakSlope = 100 * 1024
	th.MinGrowthBytes = 1024

	res := Analyze(points, nil, nil, th)
	if res.Verdict != VerdictLeak && res.Verdict != VerdictSuspect {
		t.Fatalf("verdict=%s reason=%s, want leak or suspect", res.Verdict, res.Reason)
	}
}

func TestAnalyzePlateauOK(t *testing.T) {
	points := syntheticSeries(t, 10, func(i int) int64 {
		if i < 3 {
			return int64(i) * 1024 * 1024
		}
		return 3 * 1024 * 1024
	}, "cache.(*LRU).Add")

	th := DefaultThresholds()
	th.Warmup = 0
	th.MinGrowthBytes = 1024
	th.SuspectSlope = 50 * 1024

	res := Analyze(points, nil, nil, th)
	if res.Verdict == VerdictLeak {
		t.Fatalf("plateau classified as leak: %s", res.Reason)
	}
	if res.Verdict != VerdictOK && res.Verdict != VerdictExpected {
		// plateau with residual slope may be ok
		for _, s := range res.Stacks {
			if s.Verdict == VerdictLeak {
				t.Fatalf("stack leak on plateau: %+v", s)
			}
		}
	}
}

func TestAnalyzeAllowlistExpected(t *testing.T) {
	points := syntheticSeries(t, 8, func(i int) int64 {
		return int64(i) * 1024 * 1024
	}, "myapp/internal/cache.Set")

	al := &allowlist.Config{Patterns: []string{"cache.Set", "internal/cache"}}
	th := DefaultThresholds()
	th.Warmup = 0
	th.MinGrowthBytes = 1024
	th.SuspectSlope = 10 * 1024
	th.LeakSlope = 50 * 1024
	th.MinWindow = 10 * time.Second

	res := Analyze(points, nil, al, th)
	if res.Verdict != VerdictExpected && res.Verdict != VerdictOK {
		// allowlisted stacks should not produce leak
		for _, s := range res.Stacks {
			if s.Allowlisted && (s.Verdict == VerdictLeak || s.Verdict == VerdictSuspect) {
				t.Fatalf("allowlisted stack got %s", s.Verdict)
			}
		}
	}
	found := false
	for _, s := range res.Stacks {
		if s.Allowlisted && s.Verdict == VerdictExpected {
			found = true
		}
	}
	if !found && res.Verdict != VerdictExpected {
		t.Fatalf("expected allowlisted stack; verdict=%s stacks=%v", res.Verdict, res.Stacks)
	}
}

func TestAnalyzeLoadCorrelationExpected(t *testing.T) {
	points := syntheticSeries(t, 8, func(i int) int64 {
		return int64(i) * 1024 * 1024
	}, "handler.Serve")

	var loadPts []LoadPoint
	for _, p := range points {
		// load grows with memory
		idx := int(p.Time.Sub(points[0].Time) / (5 * time.Second))
		loadPts = append(loadPts, LoadPoint{Time: p.Time, Value: float64(idx+1) * 100})
	}

	th := DefaultThresholds()
	th.Warmup = 0
	th.MinGrowthBytes = 1024
	th.SuspectSlope = 10 * 1024
	th.LeakSlope = 50 * 1024
	th.LoadCorrMin = 0.9
	th.LoadStableRatio = 0.01

	res := Analyze(points, loadPts, nil, th)
	if res.Verdict != VerdictExpected && res.Verdict != VerdictOK {
		for _, s := range res.Stacks {
			if s.Verdict == VerdictExpected {
				return
			}
		}
		t.Fatalf("want expected (load-correlated), got %s (%s)", res.Verdict, res.Reason)
	}
}

func TestSlopeAndPearson(t *testing.T) {
	xs := []float64{0, 1, 2, 3, 4}
	ys := []float64{0, 10, 20, 30, 40}
	if got := slope(xs, ys); got < 9.9 || got > 10.1 {
		t.Fatalf("slope=%v want 10", got)
	}
	if got := pearson(ys, ys); got < 0.99 {
		t.Fatalf("pearson=%v want ~1", got)
	}
}

func syntheticSeries(t *testing.T, n int, bytesAt func(i int) int64, fn string) []sample.Point {
	t.Helper()
	const step = 5 * time.Second
	start := time.Unix(1_700_000_000, 0).UTC()
	var points []sample.Point
	key := profile.StackKey(fn)
	frames := []profile.Frame{{Function: fn, File: "demo.go", Line: 1}}
	for i := 0; i < n; i++ {
		b := bytesAt(i)
		points = append(points, sample.Point{
			Time:  start.Add(time.Duration(i) * step),
			Index: i,
			Snap: &profile.Snapshot{
				TotalBytes: b,
				Stacks: map[profile.StackKey]*profile.StackInuse{
					key: {Key: key, Frames: frames, Bytes: b},
				},
			},
		})
	}
	return points
}
