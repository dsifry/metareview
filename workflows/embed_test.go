package workflows

import (
	"strings"
	"testing"
)

func TestNamesAndRead(t *testing.T) {
	names := Names()
	if len(names) != 3 || names[0] != "review-loop" || names[1] != "sdlc-loop" || names[2] != "sdlc-loop-proved" {
		t.Fatalf("Names = %v", names)
	}
	for _, n := range names {
		b, err := Read(n)
		if err != nil || !strings.HasPrefix(string(b), "workflow: "+n+"\n") {
			t.Fatalf("Read(%s): %v %q", n, err, b)
		}
	}
	for _, bad := range []string{"missing", "../sdlc-loop", "sdlc-loop.yaml", "a/b"} {
		if _, err := Read(bad); err == nil {
			t.Fatalf("Read(%q) must fail", bad)
		}
	}
}
