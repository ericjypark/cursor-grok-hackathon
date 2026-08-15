package input

import "testing"

func TestNormalizeWebsite(t *testing.T) {
	cases := map[string]string{
		"acme.dev":             "https://acme.dev",
		"acme.dev/":            "https://acme.dev",
		"https://acme.dev///":  "https://acme.dev",
		"http://acme.dev":      "http://acme.dev",
		"  https://acme.dev  ": "https://acme.dev",
		"":                     "",
	}
	for in, want := range cases {
		if got := NormalizeWebsite(in); got != want {
			t.Errorf("NormalizeWebsite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRepo(t *testing.T) {
	cases := map[string]string{
		"getcursor/cursor":                              "getcursor/cursor",
		"https://github.com/getcursor/cursor":           "getcursor/cursor",
		"https://github.com/getcursor/cursor.git":       "getcursor/cursor",
		"github.com/getcursor/cursor/":                  "getcursor/cursor",
		"https://github.com/getcursor/cursor/tree/main": "getcursor/cursor",
		"":    "",
		"   ": "",
	}
	for in, want := range cases {
		got, err := NormalizeRepo(in)
		if err != nil {
			t.Errorf("NormalizeRepo(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRepoRejectsGarbage(t *testing.T) {
	for _, in := range []string{"nope", "/", "owner/"} {
		if _, err := NormalizeRepo(in); err == nil {
			t.Errorf("NormalizeRepo(%q) should have failed", in)
		}
	}
}

func TestValidateWebsite(t *testing.T) {
	for _, ok := range []string{"acme.dev", "https://acme.dev/x"} {
		if err := ValidateWebsite(ok); err != nil {
			t.Errorf("ValidateWebsite(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "localhost", "not a url"} {
		if err := ValidateWebsite(bad); err == nil {
			t.Errorf("ValidateWebsite(%q) should have failed", bad)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"  Field Note!! ": "field-note",
		"Cursor":          "cursor",
		"a//b":            "a-b",
		"!!!":             "product",
		"":                "product",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
