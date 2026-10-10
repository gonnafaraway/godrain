package integration_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gonnafaraway/godrain/internal/analyze"
	"github.com/gonnafaraway/godrain/internal/pprofclient"
	"github.com/gonnafaraway/godrain/internal/sample"
)

func TestWatchDistinguishesLeakFromBoundedCache(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	root := repoRoot(t)
	leaky := startDemo(t, filepath.Join(root, "testdata", "leaky"))
	cached := startDemo(t, filepath.Join(root, "testdata", "cached"))

	th := analyze.DefaultThresholds()
	th.Warmup = 3 * time.Second
	th.MinGrowthBytes = 64 * 1024
	th.SuspectSlope = 8 * 1024
	th.LeakSlope = 64 * 1024
	th.MinWindow = 4 * time.Second

	leakyRes := watchURL(t, leaky, th)
	cachedRes := watchURL(t, cached, th)

	if leakyRes.Verdict != analyze.VerdictSuspect && leakyRes.Verdict != analyze.VerdictLeak {
		t.Fatalf("leaky verdict=%s want suspect|leak", leakyRes.Verdict)
	}
	if leakyRes.TotalGrowth < 1<<20 {
		t.Fatalf("leaky total growth too small: %d", leakyRes.TotalGrowth)
	}

	// Bounded cache may still show allocator noise, but retained growth must be
	// far below the unbounded demo under the same thresholds/window.
	if cachedRes.TotalGrowth > leakyRes.TotalGrowth/5 {
		t.Fatalf("cached growth=%d not << leaky growth=%d (verdict=%s)",
			cachedRes.TotalGrowth, leakyRes.TotalGrowth, cachedRes.Verdict)
	}
	if cachedRes.TotalSlope > leakyRes.TotalSlope/5 {
		t.Fatalf("cached slope=%.0f not << leaky slope=%.0f (verdict=%s)",
			cachedRes.TotalSlope, leakyRes.TotalSlope, cachedRes.Verdict)
	}
}

func watchURL(t *testing.T, base string, th analyze.Thresholds) analyze.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := pprofclient.New(base)
	if err := client.WaitReady(ctx, 100*time.Millisecond); err != nil {
		t.Fatalf("pprof ready %s: %v", base, err)
	}

	points, err := sample.New(client).Collect(ctx, sample.Options{
		Interval: time.Second,
		Duration: 10 * time.Second,
		GC:       true,
	})
	if err != nil && len(points) < 3 {
		t.Fatalf("collect %s: %v", base, err)
	}
	res := analyze.Analyze(points, nil, nil, th)
	t.Logf("%s verdict=%s growth=%d slope=%.0f stacks=%d reason=%s",
		base, res.Verdict, res.TotalGrowth, res.TotalSlope, len(res.Stacks), res.Reason)
	return res
}

func startDemo(t *testing.T, pkg string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	binName := "demo"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(t.TempDir(), binName)
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = pkg
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GODRAIN_DEMO_ADDR="+addr)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", pkg, err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})

	base := "http://" + addr
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer readyCancel()
	if err := pprofclient.New(base).WaitReady(readyCtx, 200*time.Millisecond); err != nil {
		t.Fatalf("demo not ready %s: %v", base, err)
	}
	return base
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(fmt.Errorf("go.mod not found from %s", wd))
		}
		dir = parent
	}
}
