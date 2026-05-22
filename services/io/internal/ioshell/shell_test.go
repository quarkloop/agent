package ioshell

import "testing"

func TestExecuteRequiresApproval(t *testing.T) {
	_, _, err := Execute("echo ok", false)
	if err != ErrNotApproved {
		t.Fatalf("err = %v", err)
	}
}

func TestExecuteRequiresCommand(t *testing.T) {
	_, _, err := Execute("   ", true)
	if err == nil {
		t.Fatal("expected empty command error")
	}
}

func TestExecuteRunsCommand(t *testing.T) {
	out, code, err := Execute("echo quark-ok", true)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !contains(out, "quark-ok") {
		t.Fatalf("output = %q", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
