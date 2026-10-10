package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gonnafaraway/godrain/internal/allowlist"
	"github.com/gonnafaraway/godrain/internal/analyze"
	"github.com/gonnafaraway/godrain/internal/load"
	"github.com/gonnafaraway/godrain/internal/pprofclient"
	"github.com/gonnafaraway/godrain/internal/profile"
	"github.com/gonnafaraway/godrain/internal/report"
	"github.com/gonnafaraway/godrain/internal/sample"
)

// Set via -ldflags at release build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// errFailVerdict signals that analysis completed with suspect/leak and CI should fail.
var errFailVerdict = errors.New("uncontrolled heap retention detected")

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "watch":
		err = cmdWatch(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(versionString())
		return
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, errFailVerdict) {
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", formatUserError(err))
		os.Exit(1)
	}
}

func versionString() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	return fmt.Sprintf("godrain %s (commit=%s date=%s)", v, commit, date)
}

func formatUserError(err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "pprof not ready"):
		return msg + "\nhint: expose pprof on the target (import _ \"net/http/pprof\") and bind it privately, e.g. 127.0.0.1:6060; verify with curl http://127.0.0.1:6060/debug/pprof/heap"
	case strings.Contains(lower, "empty heap profile"),
		strings.Contains(lower, "status"):
		return msg + "\nhint: confirm the URL points at a Go pprof base (http://host:port) and that /debug/pprof/heap is reachable"
	default:
		return msg
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `godrain — runtime detection of uncontrolled Go heap retention

Usage:
  godrain watch --url URL [flags]
  godrain run   --url URL [flags] -- <command> [args...]
  godrain diff  <before.pb.gz> <after.pb.gz>
  godrain version

Examples:
  godrain watch --url http://127.0.0.1:6060 --warmup 1m --duration 5m --interval 15s
  godrain run --url http://127.0.0.1:6060 --duration 2m -- ./myapp
  godrain watch --url http://127.0.0.1:6060 --allowlist configs/allowlist.example.yaml --json

Flags (watch/run):
  --url              base URL of pprof (e.g. http://127.0.0.1:6060)
  --duration         observation window (default 2m)
  --interval         sample interval (default 10s)
  --warmup           ignore growth during warmup (default 15s)
  --allowlist        YAML allowlist file
  --save-dir         directory to store raw heap profiles
  --load-url         URL returning a scalar load metric (or Prometheus text)
  --load-metric      Prometheus metric name when --load-url returns exposition format
  --json             emit JSON report
  --min-growth       min stack growth in bytes (default 65536)
  --suspect-slope    bytes/sec threshold for suspect (default 8192)
  --leak-slope       bytes/sec threshold for leak (default 65536)
  --plateau-epsilon  relative plateau sensitivity (default 0.08)
  --no-exit-code     do not exit 1 on suspect/leak

Exit codes:
  0  ok or expected
  1  suspect, leak, or tool error
  2  usage error

Security:
  Keep /debug/pprof on localhost or a private network only. See SECURITY.md.
`)
}

type watchFlags struct {
	url            string
	duration       time.Duration
	interval       time.Duration
	warmup         time.Duration
	allowlist      string
	saveDir        string
	loadURL        string
	loadMetric     string
	json           bool
	minGrowth      int64
	suspectSlope   float64
	leakSlope      float64
	plateauEpsilon float64
	exitCode       bool
}

func defaultWatchFlags() watchFlags {
	return watchFlags{
		duration:       2 * time.Minute,
		interval:       10 * time.Second,
		warmup:         15 * time.Second,
		minGrowth:      64 * 1024,
		suspectSlope:   8 * 1024,
		leakSlope:      64 * 1024,
		plateauEpsilon: 0.08,
		exitCode:       true,
	}
}

func parseWatchFlags(args []string) (watchFlags, []string, error) {
	f := defaultWatchFlags()
	var rest []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = args[i+1:]
			break
		}

		name, value, inline := splitFlag(a)
		if !inline && needsValue(name) {
			if i+1 >= len(args) {
				return f, nil, fmt.Errorf("flag %s requires a value", name)
			}
			i++
			value = args[i]
		}

		switch name {
		case "--url":
			f.url = value
		case "--duration":
			d, err := time.ParseDuration(value)
			if err != nil {
				return f, nil, err
			}
			f.duration = d
		case "--interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return f, nil, err
			}
			f.interval = d
		case "--warmup":
			d, err := time.ParseDuration(value)
			if err != nil {
				return f, nil, err
			}
			f.warmup = d
		case "--allowlist":
			f.allowlist = value
		case "--save-dir":
			f.saveDir = value
		case "--load-url":
			f.loadURL = value
		case "--load-metric":
			f.loadMetric = value
		case "--json":
			f.json = true
		case "--min-growth":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return f, nil, err
			}
			f.minGrowth = v
		case "--suspect-slope":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return f, nil, err
			}
			f.suspectSlope = v
		case "--leak-slope":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return f, nil, err
			}
			f.leakSlope = v
		case "--plateau-epsilon":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return f, nil, err
			}
			f.plateauEpsilon = v
		case "--no-exit-code":
			f.exitCode = false
		case "--exit-code":
			f.exitCode = value == "true" || value == "1"
		default:
			return f, nil, fmt.Errorf("unknown flag %q", a)
		}
	}
	return f, rest, nil
}

func splitFlag(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") {
		if eq := strings.IndexByte(arg, '='); eq >= 0 {
			return arg[:eq], arg[eq+1:], true
		}
	}
	return arg, "", false
}

func needsValue(name string) bool {
	switch name {
	case "--url", "--duration", "--interval", "--warmup",
		"--allowlist", "--save-dir", "--load-url", "--load-metric",
		"--min-growth", "--suspect-slope", "--leak-slope", "--plateau-epsilon",
		"--exit-code":
		return true
	default:
		return false
	}
}

func cmdWatch(args []string) error {
	f, _, err := parseWatchFlags(args)
	if err != nil {
		return err
	}
	if f.url == "" {
		return fmt.Errorf("--url is required")
	}
	return runWatch(f)
}

func cmdRun(args []string) error {
	f, rest, err := parseWatchFlags(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("run requires a command after --")
	}
	if f.url == "" {
		f.url = envOr("GODRAIN_PPROF_URL", "http://127.0.0.1:6060")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := exec.CommandContext(ctx, rest[0], rest[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	client := pprofclient.New(f.url)
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	fmt.Fprintf(os.Stderr, "waiting for pprof at %s ...\n", f.url)
	if err := client.WaitReady(readyCtx, 200*time.Millisecond); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	watchErr := runWatchWithContext(ctx, f, client)
	_ = cmd.Process.Signal(os.Interrupt)
	waitErr := cmd.Wait()
	if watchErr != nil {
		return watchErr
	}
	if waitErr != nil && ctx.Err() == nil {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return waitErr
		}
	}
	return nil
}

func runWatch(f watchFlags) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWatchWithContext(ctx, f, pprofclient.New(f.url))
}

func runWatchWithContext(ctx context.Context, f watchFlags, client *pprofclient.Client) error {
	var al *allowlist.Config
	if f.allowlist != "" {
		cfg, err := allowlist.Load(f.allowlist)
		if err != nil {
			return fmt.Errorf("allowlist: %w", err)
		}
		al = cfg
	}

	th := analyze.DefaultThresholds()
	th.Warmup = f.warmup
	th.MinGrowthBytes = f.minGrowth
	th.SuspectSlope = f.suspectSlope
	th.LeakSlope = f.leakSlope
	th.PlateauEpsilon = f.plateauEpsilon

	var loadPoints []analyze.LoadPoint
	var loadFetcher load.Fetcher
	if f.loadURL != "" {
		loadFetcher = &load.HTTPFetcher{URL: f.loadURL, MetricName: f.loadMetric}
	}

	sampler := sample.New(client)
	fmt.Fprintf(os.Stderr, "sampling %s for %s every %s (gc=1)\n", f.url, f.duration, f.interval)

	points, err := sampler.Collect(ctx, sample.Options{
		Interval: f.interval,
		Duration: f.duration,
		GC:       true,
		SaveDir:  f.saveDir,
		OnSample: func(p sample.Point) {
			fmt.Fprintf(os.Stderr, "  sample #%d total_inuse=%s\n", p.Index, report.FormatBytes(p.Snap.TotalBytes))
			if loadFetcher == nil {
				return
			}
			lp, lerr := load.SampleOnce(ctx, loadFetcher)
			if lerr != nil {
				fmt.Fprintf(os.Stderr, "  load metric warning: %v\n", lerr)
				return
			}
			loadPoints = append(loadPoints, lp)
		},
	})
	if err != nil && len(points) < 2 {
		return err
	}

	res := analyze.Analyze(points, loadPoints, al, th)
	if f.json {
		if err := report.WriteJSON(os.Stdout, res); err != nil {
			return err
		}
	} else if err := report.WriteText(os.Stdout, res); err != nil {
		return err
	}

	if f.exitCode && (res.Verdict == analyze.VerdictSuspect || res.Verdict == analyze.VerdictLeak) {
		return errFailVerdict
	}
	return nil
}

func cmdDiff(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("diff requires two profile files")
	}
	beforeData, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	afterData, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	before, err := profile.ParseHeap(beforeData)
	if err != nil {
		return fmt.Errorf("before: %w", err)
	}
	after, err := profile.ParseHeap(afterData)
	if err != nil {
		return fmt.Errorf("after: %w", err)
	}

	type item struct {
		delta  int64
		frames []profile.Frame
	}
	items := make([]item, 0)
	for k, d := range profile.DiffBytes(before, after) {
		if d == 0 {
			continue
		}
		var frames []profile.Frame
		if s, ok := after.Stacks[k]; ok {
			frames = s.Frames
		} else if s, ok := before.Stacks[k]; ok {
			frames = s.Frames
		}
		items = append(items, item{delta: d, frames: frames})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].delta > items[j].delta
	})

	fmt.Printf("total inuse: %s → %s (Δ %s)\n\n",
		report.FormatBytes(before.TotalBytes),
		report.FormatBytes(after.TotalBytes),
		report.FormatBytes(after.TotalBytes-before.TotalBytes))

	limit := 20
	if len(items) < limit {
		limit = len(items)
	}
	for i := 0; i < limit; i++ {
		it := items[i]
		fmt.Printf("#%d Δ %s\n%s\n", i+1, report.FormatBytes(it.delta), profile.FormatStack(it.frames, 6))
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
