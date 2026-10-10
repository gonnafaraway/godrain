package allowlist

import (
	"testing"

	"github.com/gonnafaraway/godrain/internal/profile"
)

func TestMatchSubstringAndGlob(t *testing.T) {
	c := &Config{Patterns: []string{"internal/cache", "lru.*"}}
	frames := []profile.Frame{
		{Function: "github.com/acme/app/internal/cache.Set", File: "/x/internal/cache/cache.go"},
	}
	if !c.Match(frames) {
		t.Fatal("expected substring match")
	}
	frames2 := []profile.Frame{{Function: "lru.Add"}}
	// filepath.Match("lru.*", "lru.Add") works
	if !c.Match(frames2) {
		t.Fatal("expected glob match on lru.Add")
	}
	frames3 := []profile.Frame{{Function: "main.work"}}
	if c.Match(frames3) {
		t.Fatal("unexpected match")
	}
}
