// Command fieldnote runs T0 product understanding against the field-note
// backend and writes a reviewable dossier.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/input"
	"github.com/ericjypark/cursor-grok-hackathon/internal/output"
	"github.com/ericjypark/cursor-grok-hackathon/internal/render"
	"github.com/ericjypark/cursor-grok-hackathon/internal/ui"
)

var (
	bold = lipgloss.NewStyle().Bold(true)
	dim  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	good = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("error: "+err.Error()))
		os.Exit(1)
	}
}

func run() error {
	var (
		url     = flag.String("url", "", "product website (required)")
		name    = flag.String("name", "", "product name (optional; derived from the site if omitted)")
		repo    = flag.String("repo", "", "GitHub repo as owner/repo or a full URL (optional)")
		details = flag.String("details", "", "free-text detail form (optional)")
		backend = flag.String("backend", envOr("FIELDNOTE_BACKEND", "http://127.0.0.1:8000"), "backend base URL")
		asJSON  = flag.Bool("json", false, "print the dossier as JSON and skip the interactive UI")
		outDir  = flag.String("out", "out", "directory to write results into")
	)
	flag.Parse()

	req := client.Request{Website: *url, Name: *name, Repo: *repo, Form: *details}

	// Prompting and the animated UI both need a terminal. Piped or redirected
	// output falls back to the quiet path rather than failing inside the TUI.
	interactive := isTTY() && !*asJSON

	if interactive {
		collected, err := input.Collect(req)
		if err != nil {
			return err
		}
		req = collected
	} else {
		req.Website = input.NormalizeWebsite(req.Website)
		normalized, err := input.NormalizeRepo(req.Repo)
		if err != nil {
			return err
		}
		req.Repo = normalized
		if err := input.ValidateWebsite(req.Website); err != nil {
			return fmt.Errorf("--url is required when not running in a terminal: %w", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	events, err := client.Stream(ctx, *backend, req)
	if err != nil {
		return err
	}

	var dossier client.ProductDossier
	if interactive {
		dossier, err = ui.Run(ctx, events)
	} else {
		dossier, err = consumeQuietly(events)
	}
	if err != nil {
		return err
	}

	slug := dossier.Identity.Slug
	if slug == "" {
		slug = input.Slug(dossier.Identity.CanonicalName)
	}
	jsonPath, mdPath, err := output.Write(*outDir, slug, dossier)
	if err != nil {
		return err
	}

	if *asJSON {
		blob, _ := json.MarshalIndent(dossier, "", "  ")
		fmt.Println(string(blob))
		return nil
	}
	printSummary(dossier, jsonPath, mdPath)
	return nil
}

// consumeQuietly drains the stream without a TUI, reporting progress on stderr
// so stdout stays clean for the JSON payload.
func consumeQuietly(events <-chan client.Event) (client.ProductDossier, error) {
	var dossier client.ProductDossier
	found := false
	for ev := range events {
		switch ev.Kind {
		case "stage":
			if s, err := ev.Stage(); err == nil && s.Status == "done" {
				fmt.Fprintf(os.Stderr, "· %s %s\n", s.Stage, strings.TrimSpace(deref(s.Detail)))
			}
		case "error":
			e, err := ev.Err()
			if err != nil {
				continue
			}
			if e.Fatal != nil && *e.Fatal {
				return dossier, fmt.Errorf("%s", e.Detail)
			}
			fmt.Fprintf(os.Stderr, "! %s\n", e.Detail)
		case "result":
			d, err := ev.Result()
			if err != nil {
				return dossier, fmt.Errorf("malformed result event: %w", err)
			}
			dossier, found = d, true
		}
	}
	if !found {
		return dossier, fmt.Errorf("backend closed the stream without returning a dossier")
	}
	return dossier, nil
}

func printSummary(d client.ProductDossier, jsonPath, mdPath string) {
	score := d.Disambiguation.AmbiguityScore
	collisions := 0
	if d.Disambiguation.NameCollisions != nil {
		collisions = len(*d.Disambiguation.NameCollisions)
	}

	fmt.Println(bold.Render(d.Identity.CanonicalName) + dim.Render("  "+deref(d.What.Category)))
	fmt.Printf("\n  %s %.2f  %s\n", bold.Render("Ambiguity"), score, dim.Render(render.AmbiguityVerdict(score)))

	style := good
	if score >= 0.4 {
		style = warn
	}
	fmt.Printf("  %s\n", style.Render(fmt.Sprintf("%d other thing(s) sharing this name were observed", collisions)))

	if d.Disambiguation.NameCollisions != nil {
		for _, c := range *d.Disambiguation.NameCollisions {
			fmt.Printf("    %s %s %s\n", dim.Render("–"), c.Name, dim.Render(c.WhatItIs))
		}
	}
	if d.Vocabulary.FeatureJargon != nil && len(*d.Vocabulary.FeatureJargon) > 0 {
		fmt.Printf("\n  %s %s\n", bold.Render("Distinctive terms"), dim.Render(strings.Join(*d.Vocabulary.FeatureJargon, ", ")))
	}
	if d.Provenance.DegradedSources != nil && len(*d.Provenance.DegradedSources) > 0 {
		fmt.Printf("\n  %s\n", warn.Render(fmt.Sprintf("%d source(s) degraded or dropped — see product.md", len(*d.Provenance.DegradedSources))))
	}
	fmt.Printf("\n  %s\n  %s\n\n", dim.Render(mdPath), dim.Render(jsonPath))
}

// isTTY reports whether stdout is a terminal rather than a pipe or file.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
