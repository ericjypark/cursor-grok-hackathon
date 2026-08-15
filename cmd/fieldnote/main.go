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

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/input"
	"github.com/ericjypark/cursor-grok-hackathon/internal/output"
	"github.com/ericjypark/cursor-grok-hackathon/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "\n  "+ui.Bad.Render("✗ "+err.Error())+"\n")
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
	fmt.Print(ui.Summary(dossier, jsonPath, mdPath))
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
				line := string(s.Stage)
				if detail := strings.TrimSpace(deref(s.Detail)); detail != "" {
					line += " " + detail
				}
				fmt.Fprintln(os.Stderr, "· "+line)
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
