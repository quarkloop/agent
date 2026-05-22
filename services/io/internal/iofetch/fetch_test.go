package iofetch

import (
	"context"
	"strings"
	"testing"
)

func TestFetchRejectsPrivateHost(t *testing.T) {
	result := Fetch(context.Background(), "http://127.0.0.1/", "", 1024, 5, 0)
	if result.Error == "" || !strings.Contains(result.Error, "not allowed") {
		t.Fatalf("expected private host error, got %+v", result)
	}
}

func TestFetchRejectsIPv6PrivateHost(t *testing.T) {
	result := Fetch(context.Background(), "http://[fc00::1]/", "", 1024, 5, 0)
	if result.Error == "" || !strings.Contains(result.Error, "not allowed") {
		t.Fatalf("expected private host error, got %+v", result)
	}
}

func TestFetchRejectsUnsupportedScheme(t *testing.T) {
	result := Fetch(context.Background(), "file:///etc/passwd", "", 1024, 5, 0)
	if result.Error == "" || !strings.Contains(result.Error, "http") {
		t.Fatalf("expected scheme error, got %+v", result)
	}
}

func TestFetchRequiresURL(t *testing.T) {
	result := Fetch(context.Background(), "", "", 1024, 5, 0)
	if result.Error != "url is required" {
		t.Fatalf("error = %q", result.Error)
	}
}
