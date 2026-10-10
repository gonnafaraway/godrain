package profile

import (
	"bytes"
	"testing"

	"github.com/google/pprof/profile"
)

func TestParseHeapAggregatesInuse(t *testing.T) {
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "alloc_objects", Unit: "count"},
			{Type: "alloc_space", Unit: "bytes"},
			{Type: "inuse_objects", Unit: "count"},
			{Type: "inuse_space", Unit: "bytes"},
		},
		Mapping: []*profile.Mapping{{ID: 1}},
	}
	fn := &profile.Function{ID: 1, Name: "main.leak", Filename: "main.go"}
	p.Function = []*profile.Function{fn}
	loc := &profile.Location{
		ID:      1,
		Mapping: p.Mapping[0],
		Line:    []profile.Line{{Function: fn, Line: 10}},
	}
	p.Location = []*profile.Location{loc}
	p.Sample = []*profile.Sample{
		{Location: []*profile.Location{loc}, Value: []int64{1, 100, 2, 4096}},
		{Location: []*profile.Location{loc}, Value: []int64{1, 100, 1, 2048}},
	}

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ParseHeap(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalBytes != 6144 {
		t.Fatalf("total=%d want 6144", snap.TotalBytes)
	}
	if len(snap.Stacks) != 1 {
		t.Fatalf("stacks=%d", len(snap.Stacks))
	}
}

func TestKeyAndDiff(t *testing.T) {
	frames := []Frame{{Function: "a"}, {Function: "b"}}
	key := keyFromFrames(frames)
	if key != StackKey("a\nb") {
		t.Fatalf("key=%q", key)
	}
	before := &Snapshot{Stacks: map[StackKey]*StackInuse{
		key: {Key: key, Frames: frames, Bytes: 100},
	}, TotalBytes: 100}
	after := &Snapshot{Stacks: map[StackKey]*StackInuse{
		key: {Key: key, Frames: frames, Bytes: 250},
	}, TotalBytes: 250}
	d := DiffBytes(before, after)
	if d[key] != 150 {
		t.Fatalf("delta=%d", d[key])
	}
}
