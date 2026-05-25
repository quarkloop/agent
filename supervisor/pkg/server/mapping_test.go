package server

import "testing"

func TestCloneBytesReturnsOwnedCopy(t *testing.T) {
	in := []byte("space-config")
	out := cloneBytes(in)
	if string(out) != "space-config" {
		t.Fatalf("clone = %q", out)
	}
	in[0] = 'Q'
	if string(out) != "space-config" {
		t.Fatalf("clone changed after input mutation: %q", out)
	}
}
