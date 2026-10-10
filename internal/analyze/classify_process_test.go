package analyze

import (
	"testing"
	"time"

	"github.com/gonnafaraway/godrain/internal/profile"
)

func TestClassifyProcessUsesRankNotStringOrder(t *testing.T) {
	// String order would make "ok" > "leak", which must not block escalation.
	th := DefaultThresholds()
	th.MinGrowthBytes = 1024
	th.LeakSlope = 100
	th.MinWindow = time.Second

	res := Result{
		TotalGrowth: 10 * 1024 * 1024,
		TotalSlope:  200,
		Window:      2 * time.Minute,
	}
	stacks := []StackResult{{
		Verdict: VerdictSuspect,
		Reason:  "sustained retained growth without plateau",
		Frames:  []profile.Frame{{Function: "main.leak"}},
	}}
	got, _ := classifyProcess(stacks, res, th)
	if got != VerdictLeak {
		t.Fatalf("verdict=%s want leak", got)
	}
}
