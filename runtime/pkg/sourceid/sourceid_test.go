package sourceid

import "testing"

func TestCanonicalEquatesLocalFileURIAndAbsolutePath(t *testing.T) {
	if !Equal("/tmp/uploads/source.pdf", "file:///tmp/uploads/source.pdf") {
		t.Fatal("local path and file URI should identify the same source")
	}
	if !Equal("/tmp/uploads/../uploads/source.pdf", "file://localhost/tmp/uploads/source.pdf") {
		t.Fatal("clean local path and localhost file URI should identify the same source")
	}
}

func TestCanonicalPreservesNonFileURIIdentity(t *testing.T) {
	if Equal("https://files.example/source.pdf", "file:///source.pdf") {
		t.Fatal("remote and local sources must remain distinct")
	}
}
