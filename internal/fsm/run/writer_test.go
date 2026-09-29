package run

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewerWriter(t *testing.T) {
	for _, c := range []struct {
		writer, reader string
		want           bool
	}{
		{"", "0.14.0", false},               // written before 0.14
		{"0.14.0", "0.14.0", false},         // same version
		{"0.14.9", "0.14.0", false},         // a patch never changes the format
		{"0.13.4", "0.14.0", false},         // older writer
		{"0.15.0", "0.14.2", true},          // newer minor
		{"1.0.0", "0.99.0", true},           // newer major
		{"v0.15.0-rc1", "0.14.0", true},     // a leading v and a pre-release suffix parse
		{"0.14.0", "v0.14.0", false},        //
		{"garbage", "0.14.0", true},         // an unparseable writer is refused, never guessed at
		{"1", "0.14.0", true},               //
		{"0.x.0", "0.14.0", true},           //
		{"0.14.0", "not-a-version", true},   // nor is an unparseable reader trusted
		{"0.13.0", "1.0.0", false},          //
		{"0.13.0", "0.13.0-dirty", false},   //
		{"0.14.0", "0.13.99-dirty", true},   //
		{"x.14.0", "0.14.0", true},          //
		{"0.14.0", "0.y.0", true},           //
		{"0.14.0", "0", true},               //
		{"2.0.0", "10.0.0", false},          // numeric, not lexical
		{"10.0.0", "9.99.0", true},          //
		{"0.9.0", "0.10.0", false},          //
		{"0.10.0", "0.9.0", true},           //
		{"0.14.0.1", "0.14.0", false},       //
		{"0.14", "0.14.0", false},           //
		{"0.15", "0.14.0", true},            //
		{"0.14.0", "0.14", false},           //
		{"0.14.0", "0.13", true},            //
		{"0.14.0", "v", true},               //
		{"v", "0.14.0", true},               //
		{"0.14.0", "", true},                // an empty reader is not a version
		{"0.14.0", "0.14.0+build.5", false}, //
	} {
		if got := newerWriter(c.writer, c.reader); got != c.want {
			t.Errorf("newerWriter(%q, %q) = %v, want %v", c.writer, c.reader, got, c.want)
		}
	}
}

// AC-5.4 (#180), this binary's side: a run whose writer is a newer minor version is refused with the version
// error — never folded, never misread. A run from an older or equal writer, or one from before 0.14, folds.
func TestFoldRefusesARunFromANewerWriter(t *testing.T) {
	saved := ReaderVersion
	t.Cleanup(func() { ReaderVersion = saved })
	ReaderVersion = "0.14.0"
	for writer, refused := range map[string]bool{"": false, "0.13.4": false, "0.14.7": false, "0.15.0": true} {
		b := NewBuilder(runA)
		data := baseInit()
		data.Writer = writer
		b.Init(data)
		_, err := Fold(b.Events())
		var fe *FoldError
		switch {
		case refused && (!errors.As(err, &fe) || fe.Code != CodeAuditVersion || fe.Reason != ReasonNewerWriter):
			t.Errorf("writer %q: want %s/%s, got %v", writer, CodeAuditVersion, ReasonNewerWriter, err)
		case !refused && err != nil:
			t.Errorf("writer %q: %v", writer, err)
		}
	}
}

// #180: many first runs creating their runs at once in a fresh store all succeed. The self-ignoring .gitignore
// was written through one shared temp name, so one writer's rename could take another's temp away.
func TestConcurrentFirstRunsInAFreshStore(t *testing.T) {
	for round := 0; round < 20; round++ {
		root := t.TempDir()
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			go func(i int) {
				id := fmt.Sprintf("mrv-20260826-00000000000000%d-fsm-sdlc-loop-sdlc-loop-aaaaaaaa", i)
				b := NewBuilder(id)
				b.Init(baseInit())
				_, err := NewJSONLStore(root, Options{}).Create(id, b.Events()[0])
				errs <- err
			}(i)
		}
		for i := 0; i < 8; i++ {
			if err := <-errs; err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
}
