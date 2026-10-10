package profile

import (
	"fmt"
	"strings"

	"github.com/google/pprof/profile"
)

// StackKey is a stable identity for an allocation call stack (leaf → root).
type StackKey string

// Frame is one location in a stack.
type Frame struct {
	Function string `json:"function"`
	File     string `json:"file,omitempty"`
	Line     int64  `json:"line,omitempty"`
}

// StackInuse holds retained bytes for one stack at a point in time.
type StackInuse struct {
	Key    StackKey
	Frames []Frame
	Bytes  int64
}

// Snapshot is aggregated inuse memory by stack from one heap profile.
type Snapshot struct {
	Stacks     map[StackKey]*StackInuse
	TotalBytes int64
}

// ParseHeap parses a pprof heap profile and aggregates inuse_space by stack.
func ParseHeap(data []byte) (*Snapshot, error) {
	p, err := profile.ParseData(data)
	if err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}

	inuseBytesIdx := sampleIndex(p, "inuse_space")
	if inuseBytesIdx < 0 {
		return nil, fmt.Errorf("profile has no inuse_space sample type")
	}

	snap := &Snapshot{Stacks: make(map[StackKey]*StackInuse)}
	for _, s := range p.Sample {
		var bytes int64
		if inuseBytesIdx < len(s.Value) {
			bytes = s.Value[inuseBytesIdx]
		}
		if bytes == 0 {
			continue
		}

		frames := framesFromSample(s)
		if len(frames) == 0 {
			continue
		}
		key := keyFromFrames(frames)
		existing, ok := snap.Stacks[key]
		if !ok {
			existing = &StackInuse{Key: key, Frames: frames}
			snap.Stacks[key] = existing
		}
		existing.Bytes += bytes
		snap.TotalBytes += bytes
	}
	return snap, nil
}

func sampleIndex(p *profile.Profile, name string) int {
	for i, st := range p.SampleType {
		if st.Type == name {
			return i
		}
	}
	return -1
}

func framesFromSample(s *profile.Sample) []Frame {
	// pprof sample locations are leaf-first.
	var frames []Frame
	for _, loc := range s.Location {
		for _, line := range loc.Line {
			fn := "unknown"
			file := ""
			var lineno int64
			if line.Function != nil {
				fn = line.Function.Name
				file = line.Function.Filename
			}
			lineno = line.Line
			frames = append(frames, Frame{Function: fn, File: file, Line: lineno})
		}
	}
	return frames
}

func keyFromFrames(frames []Frame) StackKey {
	parts := make([]string, len(frames))
	for i, f := range frames {
		parts[i] = f.Function
	}
	return StackKey(strings.Join(parts, "\n"))
}

// FormatStack returns a short multi-line stack for display (top frames).
func FormatStack(frames []Frame, maxFrames int) string {
	if maxFrames <= 0 || maxFrames > len(frames) {
		maxFrames = len(frames)
	}
	var b strings.Builder
	for i := 0; i < maxFrames; i++ {
		f := frames[i]
		if f.File != "" {
			fmt.Fprintf(&b, "  %s\n    %s:%d\n", f.Function, f.File, f.Line)
		} else {
			fmt.Fprintf(&b, "  %s\n", f.Function)
		}
	}
	return b.String()
}

// DiffBytes returns per-stack byte deltas (after - before).
func DiffBytes(before, after *Snapshot) map[StackKey]int64 {
	keys := make(map[StackKey]struct{}, len(before.Stacks)+len(after.Stacks))
	for k := range before.Stacks {
		keys[k] = struct{}{}
	}
	for k := range after.Stacks {
		keys[k] = struct{}{}
	}
	out := make(map[StackKey]int64, len(keys))
	for k := range keys {
		var a, b int64
		if s, ok := after.Stacks[k]; ok {
			a = s.Bytes
		}
		if s, ok := before.Stacks[k]; ok {
			b = s.Bytes
		}
		out[k] = a - b
	}
	return out
}
