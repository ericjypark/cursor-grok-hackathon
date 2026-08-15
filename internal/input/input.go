// Package input collects and normalizes the three onboarding inputs.
package input

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/ui"
)

var (
	schemeRe   = regexp.MustCompile(`(?i)^https?://`)
	nonSlugRe  = regexp.MustCompile(`[^a-z0-9]+`)
	dashRunsRe = regexp.MustCompile(`-+`)
)

// NormalizeWebsite adds a scheme when the user omitted one and trims trailing
// slashes, so "acme.dev/" and "https://acme.dev" agree.
func NormalizeWebsite(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !schemeRe.MatchString(s) {
		s = "https://" + s
	}
	return strings.TrimRight(s, "/")
}

// NormalizeRepo accepts "owner/repo", a full GitHub URL, or a .git suffix and
// returns "owner/repo". Empty input is not an error — the repo is optional.
func NormalizeRepo(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	s = strings.TrimSuffix(strings.TrimRight(s, "/"), ".git")
	if strings.Contains(s, "github.com") {
		raw := s
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("unparseable repo %q: %w", s, err)
		}
		s = strings.Trim(u.Path, "/")
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("repo %q is not owner/repo", s)
	}
	return parts[0] + "/" + parts[1], nil
}

// ValidateWebsite is the form's inline check; the website is the one input we
// genuinely cannot proceed without.
func ValidateWebsite(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("website is required")
	}
	u, err := url.Parse(NormalizeWebsite(s))
	if err != nil || u.Host == "" || !strings.Contains(u.Host, ".") {
		return errors.New("that doesn't look like a URL")
	}
	return nil
}

// Slug derives a filesystem-safe directory name.
func Slug(name string) string {
	s := dashRunsRe.ReplaceAllString(nonSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if s = strings.Trim(s, "-"); s != "" {
		return s
	}
	return "product"
}

// Collect prompts only for what the flags left blank.
func Collect(req client.Request) (client.Request, error) {
	fields := []huh.Field{}
	if req.Website == "" {
		fields = append(fields, huh.NewInput().
			Title("Product website").
			Description("The one required input.").
			Placeholder("https://cursor.com").
			Value(&req.Website).
			Validate(ValidateWebsite))
	}
	if req.Name == "" {
		fields = append(fields, huh.NewInput().
			Title("Product name").
			Description("Optional — derived from the site if you skip it.").
			Value(&req.Name))
	}
	if req.Repo == "" {
		fields = append(fields, huh.NewInput().
			Title("GitHub repo").
			Description("Optional. owner/repo or a full URL.").
			Placeholder("getcursor/cursor").
			Value(&req.Repo).
			Validate(func(s string) error { _, err := NormalizeRepo(s); return err }))
	}
	if req.Form == "" {
		fields = append(fields, huh.NewText().
			Title("Anything else worth knowing?").
			Description("Optional, but the most precise signal we get — it tells us which product you actually mean.").
			Lines(3).
			Value(&req.Form))
	}

	if len(fields) > 0 {
		fmt.Println(ui.FormHeader())
		if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(ui.FormTheme()).Run(); err != nil {
			return req, err
		}
	}

	req.Website = NormalizeWebsite(req.Website)
	repo, err := NormalizeRepo(req.Repo)
	if err != nil {
		return req, err
	}
	req.Repo = repo
	req.Name = strings.TrimSpace(req.Name)
	req.Form = strings.TrimSpace(req.Form)
	return req, ValidateWebsite(req.Website)
}
