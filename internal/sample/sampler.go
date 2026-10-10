package sample

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gonnafaraway/godrain/internal/pprofclient"
	"github.com/gonnafaraway/godrain/internal/profile"
)

// Point is one timed heap snapshot.
type Point struct {
	Time  time.Time
	Snap  *profile.Snapshot
	Index int
}

// Options configure the sampling loop.
type Options struct {
	Interval time.Duration
	Duration time.Duration
	GC       bool
	SaveDir  string // optional directory to persist raw profiles
	OnSample func(Point)
}

// Sampler periodically fetches heap profiles.
type Sampler struct {
	Client *pprofclient.Client
}

// New returns a Sampler that uses client to fetch profiles.
func New(client *pprofclient.Client) *Sampler {
	return &Sampler{Client: client}
}

// Collect runs until duration elapses or context is cancelled.
func (s *Sampler) Collect(ctx context.Context, opt Options) ([]Point, error) {
	if opt.Interval <= 0 {
		opt.Interval = 10 * time.Second
	}
	if opt.Duration <= 0 {
		opt.Duration = 2 * time.Minute
	}

	if opt.SaveDir != "" {
		if err := os.MkdirAll(opt.SaveDir, 0o755); err != nil {
			return nil, fmt.Errorf("create save dir: %w", err)
		}
	}

	deadline := time.Now().Add(opt.Duration)
	var points []Point
	idx := 0

	take := func() error {
		data, err := s.Client.FetchHeap(ctx, opt.GC)
		if err != nil {
			return err
		}
		snap, err := profile.ParseHeap(data)
		if err != nil {
			return err
		}
		p := Point{Time: time.Now(), Snap: snap, Index: idx}
		if opt.SaveDir != "" {
			name := filepath.Join(opt.SaveDir, fmt.Sprintf("heap-%04d.pb.gz", idx))
			if err := os.WriteFile(name, data, 0o644); err != nil {
				return fmt.Errorf("save profile: %w", err)
			}
		}
		points = append(points, p)
		if opt.OnSample != nil {
			opt.OnSample(p)
		}
		idx++
		return nil
	}

	if err := take(); err != nil {
		return nil, err
	}

	ticker := time.NewTicker(opt.Interval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			if len(points) == 0 {
				return nil, ctx.Err()
			}
			return points, nil
		case <-ticker.C:
			if !time.Now().Before(deadline) {
				return points, nil
			}
			if err := take(); err != nil {
				return points, err
			}
		}
	}
	return points, nil
}
