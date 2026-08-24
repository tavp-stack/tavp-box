package certs

import (
	"path/filepath"
	"strings"
	"testing"
)

func withTempCA(t *testing.T) {
	t.Helper()
	dirOverride = filepath.Join(t.TempDir(), "ca")
	t.Cleanup(func() {
		dirOverride = ""
		cacheMu.Lock()
		cache = map[string]*keypair{}
		cacheMu.Unlock()
		trustedOK = false
	})
}

// On-demand issuance must produce a valid leaf for any subdomain depth,
// cached across calls (#26).
func TestForHostIssuesAndCaches(t *testing.T) {
	withTempCA(t)
	const suffix = "poc.invalid"

	if _, err := EnsureWildcard(suffix); err != nil && !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("unexpected error ensuring CA: %v", err)
	}

	a1, err := ForHost("app.penbill."+suffix, suffix)
	if err != nil {
		t.Fatalf("ForHost: %v", err)
	}
	a2, err := ForHost("APP.PENBILL."+suffix+":443", suffix)
	if err != nil {
		t.Fatalf("ForHost: %v", err)
	}
	if a1 != a2 {
		t.Fatal("expected cached certificate for same host")
	}

	b, err := ForHost("deep.sub."+suffix, suffix)
	if err != nil {
		t.Fatalf("ForHost deep: %v", err)
	}
	if b == a1 {
		t.Fatal("different hosts must not share certificate")
	}

	// Out-of-suffix host falls back to wildcard
	w, err := ForHost("evil.example.com", suffix)
	if err != nil {
		t.Fatalf("ForHost fallback: %v", err)
	}
	wild, err := wildcardKeypair(suffix)
	if err != nil {
		t.Fatalf("wildcardKeypair: %v", err)
	}
	if w != wild.tls {
		t.Fatal("out-of-suffix host should fall back to wildcard cert")
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"App.Penbill.X:443": "app.penbill.x",
		"app.x.":            "app.x",
		"plain.x":           "plain.x",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
