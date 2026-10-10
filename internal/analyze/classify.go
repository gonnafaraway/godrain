package analyze

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gonnafaraway/godrain/internal/allowlist"
	"github.com/gonnafaraway/godrain/internal/profile"
	"github.com/gonnafaraway/godrain/internal/sample"
)

// Verdict is the classification outcome.
type Verdict string

const (
	VerdictOK       Verdict = "ok"
	VerdictExpected Verdict = "expected"
	VerdictSuspect  Verdict = "suspect"
	VerdictLeak     Verdict = "leak"
)

// Thresholds control classification sensitivity.
type Thresholds struct {
	Warmup          time.Duration
	MinGrowthBytes  int64         // ignore stacks with total growth below this
	PlateauEpsilon  float64       // relative variance of 2nd half vs peak → plateau
	SuspectSlope    float64       // bytes/sec to mark suspect
	LeakSlope       float64       // bytes/sec to mark leak
	MinWindow       time.Duration // minimum observation after warmup for leak
	LoadCorrMin     float64       // Pearson |r| above this → expected (with load)
	LoadStableRatio float64       // load slope / mean below this → load considered stable
	TopN            int
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		Warmup:          15 * time.Second,
		MinGrowthBytes:  64 * 1024, // 64 KiB
		PlateauEpsilon:  0.08,
		SuspectSlope:    8 * 1024,  // 8 KiB/s
		LeakSlope:       64 * 1024, // 64 KiB/s
		MinWindow:       45 * time.Second,
		LoadCorrMin:     0.75,
		LoadStableRatio: 0.05,
		TopN:            15,
	}
}

// StackResult is the per-stack analysis.
type StackResult struct {
	Key         profile.StackKey `json:"key"`
	Frames      []profile.Frame  `json:"frames"`
	Verdict     Verdict          `json:"verdict"`
	SlopeBps    float64          `json:"slope_bytes_per_sec"`
	Growth      int64            `json:"growth_bytes"`
	Plateau     bool             `json:"plateau"`
	Allowlisted bool             `json:"allowlisted"`
	LoadCorr    float64          `json:"load_correlation,omitempty"`
	Reason      string           `json:"reason"`
}

// Result is the full analysis report payload.
type Result struct {
	Verdict     Verdict       `json:"verdict"`
	Reason      string        `json:"reason"`
	SampleCount int           `json:"sample_count"`
	Window      time.Duration `json:"window_ns"`
	TotalGrowth int64         `json:"total_growth_bytes"`
	TotalSlope  float64       `json:"total_slope_bytes_per_sec"`
	Stacks      []StackResult `json:"stacks"`
	HasLoad     bool          `json:"has_load"`
}

// LoadPoint is a load-metric sample used for correlation with heap retention.
type LoadPoint struct {
	Time  time.Time
	Value float64
}

var verdictRank = map[Verdict]int{
	VerdictOK:       0,
	VerdictExpected: 1,
	VerdictSuspect:  2,
	VerdictLeak:     3,
}

// Analyze classifies heap retention growth across a sample series.
func Analyze(points []sample.Point, load []LoadPoint, al *allowlist.Config, th Thresholds) Result {
	if th.TopN == 0 {
		th = DefaultThresholds()
	}
	res := Result{Verdict: VerdictOK, Reason: "insufficient data", SampleCount: len(points)}
	if len(points) < 2 {
		return res
	}

	filtered := filterWarmup(points, th.Warmup)
	if len(filtered) < 2 {
		res.Reason = "not enough samples after warmup"
		return res
	}
	res.Window = filtered[len(filtered)-1].Time.Sub(filtered[0].Time)
	res.SampleCount = len(filtered)

	// Build per-stack time series.
	type series struct {
		frames []profile.Frame
		times  []float64 // seconds from filtered[0]
		values []float64
	}
	byStack := map[profile.StackKey]*series{}
	t0 := filtered[0].Time
	for _, p := range filtered {
		sec := p.Time.Sub(t0).Seconds()
		for k, st := range p.Snap.Stacks {
			s, ok := byStack[k]
			if !ok {
				s = &series{frames: st.Frames}
				byStack[k] = s
			}
			s.times = append(s.times, sec)
			s.values = append(s.values, float64(st.Bytes))
		}
	}

	// Align missing points as 0 for stacks that appear mid-window.
	allTimes := make([]float64, len(filtered))
	for i, p := range filtered {
		allTimes[i] = p.Time.Sub(t0).Seconds()
	}
	for _, s := range byStack {
		if len(s.times) == len(allTimes) {
			continue
		}
		m := make(map[float64]float64, len(s.times))
		for i := range s.times {
			m[s.times[i]] = s.values[i]
		}
		s.times = allTimes
		s.values = make([]float64, len(allTimes))
		for i, t := range allTimes {
			s.values[i] = m[t]
		}
	}

	loadAligned := alignLoad(filtered, load)
	res.HasLoad = len(loadAligned) == len(filtered) && len(load) > 0

	var totalSeries []float64
	for _, p := range filtered {
		totalSeries = append(totalSeries, float64(p.Snap.TotalBytes))
	}
	res.TotalSlope = slope(allTimes, totalSeries)
	res.TotalGrowth = int64(totalSeries[len(totalSeries)-1] - totalSeries[0])

	var stackResults []StackResult
	for k, s := range byStack {
		growth := int64(s.values[len(s.values)-1] - s.values[0])
		// Only growing retained stacks can be uncontrolled allocations.
		if growth < th.MinGrowthBytes {
			continue
		}
		if isRuntimeNoise(s.frames) {
			continue
		}
		sl := slope(s.times, s.values)
		if sl < th.SuspectSlope*0.25 {
			continue
		}
		plat := isPlateau(s.values, th.PlateauEpsilon)
		matched := al.Match(s.frames)
		var corr float64
		if res.HasLoad {
			corr = pearson(s.values, loadAligned)
		}

		sr := StackResult{
			Key:         k,
			Frames:      s.frames,
			SlopeBps:    sl,
			Growth:      growth,
			Plateau:     plat,
			Allowlisted: matched,
			LoadCorr:    corr,
		}
		sr.Verdict, sr.Reason = classifyStack(sr, res.HasLoad, loadStable(loadAligned, th.LoadStableRatio), res.Window, th)
		if sr.Verdict == VerdictOK {
			continue
		}
		stackResults = append(stackResults, sr)
	}

	sort.Slice(stackResults, func(i, j int) bool {
		return stackResults[i].SlopeBps > stackResults[j].SlopeBps
	})
	if th.TopN > 0 && len(stackResults) > th.TopN {
		stackResults = stackResults[:th.TopN]
	}
	res.Stacks = stackResults
	res.Verdict, res.Reason = classifyProcess(stackResults, res, th)
	return res
}

func classifyStack(sr StackResult, hasLoad, loadIsStable bool, window time.Duration, th Thresholds) (Verdict, string) {
	if sr.Allowlisted {
		return VerdictExpected, "stack matches allowlist"
	}
	if sr.Growth < th.MinGrowthBytes || sr.SlopeBps <= 0 {
		return VerdictOK, "no significant retained growth"
	}
	if sr.Plateau {
		return VerdictOK, "growth leveled into a plateau"
	}
	if hasLoad && math.Abs(sr.LoadCorr) >= th.LoadCorrMin && !loadIsStable {
		return VerdictExpected, "retained growth correlates with load"
	}
	if sr.SlopeBps >= th.LeakSlope && window >= th.MinWindow {
		if hasLoad && loadIsStable {
			return VerdictLeak, "high retained growth while load is stable"
		}
		if !hasLoad {
			return VerdictLeak, "high sustained retained growth without plateau"
		}
		if math.Abs(sr.LoadCorr) < th.LoadCorrMin {
			return VerdictLeak, "high retained growth not correlated with load"
		}
	}
	if sr.SlopeBps >= th.SuspectSlope && !sr.Plateau {
		return VerdictSuspect, "sustained retained growth without plateau"
	}
	return VerdictOK, "within thresholds"
}

func classifyProcess(stacks []StackResult, res Result, th Thresholds) (Verdict, string) {
	worst := VerdictOK
	reason := "no uncontrolled retained growth detected"
	for _, s := range stacks {
		if verdictRank[s.Verdict] > verdictRank[worst] {
			worst = s.Verdict
			reason = s.Reason
			if len(s.Frames) > 0 {
				reason = s.Reason + " (" + firstAppFrame(s.Frames) + ")"
			}
		}
	}
	// Process-level leak only when total retained heap actually grew.
	if verdictRank[worst] < verdictRank[VerdictLeak] &&
		res.TotalGrowth >= th.MinGrowthBytes &&
		res.TotalSlope >= th.LeakSlope &&
		res.Window >= th.MinWindow {
		for _, s := range stacks {
			if s.Verdict == VerdictSuspect || s.Verdict == VerdictLeak {
				return VerdictLeak, "process-level retained heap growth exceeds leak threshold"
			}
		}
		if len(stacks) == 0 {
			return VerdictLeak, "process-level retained heap growth exceeds leak threshold"
		}
	}
	return worst, reason
}

func isRuntimeNoise(frames []profile.Frame) bool {
	if len(frames) == 0 {
		return true
	}
	// Skip stacks that never leave the runtime allocator / scheduler.
	for _, f := range frames {
		if !isRuntimeFunc(f.Function) {
			return false
		}
	}
	return true
}

func isRuntimeFunc(name string) bool {
	return strings.HasPrefix(name, "runtime.") ||
		strings.HasPrefix(name, "runtime/") ||
		name == "mallocgc"
}

func firstAppFrame(frames []profile.Frame) string {
	for _, f := range frames {
		if !isRuntimeFunc(f.Function) {
			return f.Function
		}
	}
	if len(frames) == 0 {
		return ""
	}
	return frames[0].Function
}

func filterWarmup(points []sample.Point, warmup time.Duration) []sample.Point {
	if warmup <= 0 || len(points) == 0 {
		return points
	}
	cut := points[0].Time.Add(warmup)
	out := make([]sample.Point, 0, len(points))
	for _, p := range points {
		if !p.Time.Before(cut) {
			out = append(out, p)
		}
	}
	// Keep at least 2 points: if warmup ate everything, use full series.
	if len(out) < 2 {
		return points
	}
	return out
}

func alignLoad(points []sample.Point, load []LoadPoint) []float64 {
	if len(load) == 0 {
		return nil
	}
	out := make([]float64, len(points))
	for i, p := range points {
		out[i] = interpolateLoad(load, p.Time)
	}
	return out
}

func interpolateLoad(load []LoadPoint, t time.Time) float64 {
	if len(load) == 1 {
		return load[0].Value
	}
	if !t.After(load[0].Time) {
		return load[0].Value
	}
	last := load[len(load)-1]
	if !t.Before(last.Time) {
		return last.Value
	}
	for i := 1; i < len(load); i++ {
		if t.Before(load[i].Time) || t.Equal(load[i].Time) {
			a, b := load[i-1], load[i]
			dt := b.Time.Sub(a.Time).Seconds()
			if dt == 0 {
				return b.Value
			}
			frac := t.Sub(a.Time).Seconds() / dt
			return a.Value + frac*(b.Value-a.Value)
		}
	}
	return last.Value
}

func slope(xs, ys []float64) float64 {
	n := len(xs)
	if n != len(ys) || n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
	}
	nf := float64(n)
	den := nf*sumXX - sumX*sumX
	if den == 0 {
		return 0
	}
	return (nf*sumXY - sumX*sumY) / den
}

func isPlateau(values []float64, epsilon float64) bool {
	n := len(values)
	if n < 4 {
		return false
	}
	half := n / 2
	second := values[half:]
	var max float64
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return true
	}
	mean := 0.0
	for _, v := range second {
		mean += v
	}
	mean /= float64(len(second))
	var variance float64
	for _, v := range second {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(second))
	rel := math.Sqrt(variance) / max
	// Also require first half to have grown more than second half moves.
	firstGrowth := values[half-1] - values[0]
	secondGrowth := values[n-1] - values[half]
	return rel <= epsilon && secondGrowth <= math.Max(firstGrowth*0.25, max*epsilon)
}

func pearson(a, b []float64) float64 {
	n := len(a)
	if n != len(b) || n < 2 {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, denA, denB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		denA += da * da
		denB += db * db
	}
	den := math.Sqrt(denA * denB)
	if den == 0 {
		return 0
	}
	return num / den
}

func loadStable(values []float64, ratio float64) bool {
	if len(values) < 2 {
		return true
	}
	xs := make([]float64, len(values))
	for i := range xs {
		xs[i] = float64(i)
	}
	sl := slope(xs, values)
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	if mean == 0 {
		return math.Abs(sl) < 1e-9
	}
	// slope per step relative to mean
	return math.Abs(sl)/math.Abs(mean) <= ratio
}
