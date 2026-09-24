package mutationfresh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestDigestOf(t *testing.T) {
	sum := sha256.Sum256([]byte("x"))
	if got := digestOf(Entry{Data: []byte("x")}); got != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("file digest %q", got)
	}
	if got := digestOf(Entry{Data: []byte("x"), Symlink: true}); got != "symlink:"+hex.EncodeToString(sum[:]) {
		t.Errorf("symlink digest %q", got)
	}
}

// Spec K3.4 and Plan 1a: the harness digests its config file as canonical JSON without `views`.
// The gate must produce the same digest byte for byte, or the config reads as changed forever.
func TestConfigDigestVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mutation-incremental/config-digest-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct{ Name, Text, Digest string } `json:"cases"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) < 5 {
		t.Fatalf("expected the shared vectors, got %d cases", len(v.Cases))
	}
	for _, c := range v.Cases {
		if got := configDigest([]byte(c.Text)); got != c.Digest {
			t.Errorf("%s: got %s, want %s", c.Name, got, c.Digest)
		}
	}
}

func TestConfigDigestOfUnparseableTextIsTheRawDigest(t *testing.T) {
	if got, want := configDigest([]byte("not json")), digestOf(Entry{Data: []byte("not json")}); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
