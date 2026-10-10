package load

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gonnafaraway/godrain/internal/analyze"
)

// Fetcher obtains a scalar load metric.
type Fetcher interface {
	Fetch(ctx context.Context) (float64, error)
}

// HTTPFetcher GETs a URL and parses the response body as a float
// (plain number or Prometheus gauge line matching MetricName).
type HTTPFetcher struct {
	URL        string
	MetricName string // optional; if set, parse Prometheus text exposition
	Client     *http.Client
}

// Fetch returns the current load metric value.
func (f *HTTPFetcher) Fetch(ctx context.Context) (float64, error) {
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("load metric status %d", resp.StatusCode)
	}
	text := strings.TrimSpace(string(body))
	if f.MetricName == "" {
		return strconv.ParseFloat(text, 64)
	}
	return parsePromGauge(text, f.MetricName)
}

func parsePromGauge(text, name string) (float64, error) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, name) {
			continue
		}
		rest := strings.TrimPrefix(line, name)
		if len(rest) > 0 && rest[0] != ' ' && rest[0] != '{' && rest[0] != '\t' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		return v, nil
	}
	return 0, fmt.Errorf("metric %q not found", name)
}

// SampleOnce fetches a single load point for alignment with a heap sample.
func SampleOnce(ctx context.Context, f Fetcher) (analyze.LoadPoint, error) {
	v, err := f.Fetch(ctx)
	if err != nil {
		return analyze.LoadPoint{}, err
	}
	return analyze.LoadPoint{Time: time.Now(), Value: v}, nil
}
