package config

import "testing"

func TestEffectivePHPVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", DefaultPHPVersion},
		{"8", DefaultPHPVersion},
		{"8.3", "8.3"},
		{"8.3.7", "8.3"},
		{"8.4", "8.4"},
		{"8.4.24", "8.4"},
		{`8.4`, "8.4"},
		{"  8.4  ", "8.4"},
		{"7.4", DefaultPHPVersion},
		{"bogus", DefaultPHPVersion},
	}

	for _, c := range cases {
		cfg := &ProjectConfig{PHPVersion: c.in}
		if got := EffectivePHPVersion(cfg); got != c.want {
			t.Errorf("EffectivePHPVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// nil config must not panic and must default
	if got := EffectivePHPVersion(nil); got != DefaultPHPVersion {
		t.Errorf("EffectivePHPVersion(nil) = %q, want %q", got, DefaultPHPVersion)
	}
}
