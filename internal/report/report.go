package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gonnafaraway/godrain/internal/analyze"
	"github.com/gonnafaraway/godrain/internal/profile"
)

// WriteJSON emits the analysis result as JSON.
func WriteJSON(w io.Writer, res analyze.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// WriteText emits a human-readable report.
func WriteText(w io.Writer, res analyze.Result) error {
	var b strings.Builder
	fmt.Fprintf(&b, "godrain verdict: %s\n", res.Verdict)
	fmt.Fprintf(&b, "reason: %s\n", res.Reason)
	fmt.Fprintf(&b, "samples: %d  window: %s  total_growth: %s  total_slope: %s/s\n",
		res.SampleCount,
		res.Window.Round(time.Second),
		FormatBytes(res.TotalGrowth),
		FormatBytes(int64(res.TotalSlope)),
	)
	if res.HasLoad {
		fmt.Fprintf(&b, "load metric: enabled\n")
	}
	fmt.Fprintf(&b, "\n")

	if len(res.Stacks) == 0 {
		fmt.Fprintf(&b, "No significant growing stacks.\n")
		_, err := io.WriteString(w, b.String())
		return err
	}

	fmt.Fprintf(&b, "Top growing stacks:\n")
	for i, s := range res.Stacks {
		fmt.Fprintf(&b, "\n#%d [%s] growth=%s slope=%s/s plateau=%v allowlisted=%v\n",
			i+1, s.Verdict, FormatBytes(s.Growth), FormatBytes(int64(s.SlopeBps)), s.Plateau, s.Allowlisted)
		if s.Reason != "" {
			fmt.Fprintf(&b, "  reason: %s\n", s.Reason)
		}
		if res.HasLoad {
			fmt.Fprintf(&b, "  load_corr: %.2f\n", s.LoadCorr)
		}
		b.WriteString(profile.FormatStack(s.Frames, 6))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// FormatBytes renders a byte count with a binary unit suffix.
func FormatBytes(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	sign := ""
	if neg {
		sign = "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%s%d B", sign, n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%s%.1f %ciB", sign, float64(n)/float64(div), "KMGTPE"[exp])
}
