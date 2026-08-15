package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

var update = flag.Bool("update", false, "rewrite golden files")

func ptr[T any](v T) *T { return &v }

func fixture() client.ProductDossier {
	return client.ProductDossier{
		Identity: client.Identity{
			CanonicalName:    "Cursor",
			Slug:             "cursor",
			Tagline:          ptr("The AI code editor"),
			Homepage:         ptr("https://cursor.com"),
			Repo:             ptr("getcursor/cursor"),
			Aliases:          ptr([]string{"Cursor IDE", "Cursor editor"}),
			CliCommands:      ptr([]string{"cursor"}),
			PackageNames:     ptr([]client.PackageRef{{Registry: "npm", Name: "cursor"}}),
			OfficialAccounts: ptr(map[string]string{"x": "cursor_ai"}),
		},
		What: client.What{
			Category:        ptr("AI code editor"),
			Description:     ptr("An editor built for programming with AI."),
			PrimaryUseCases: ptr([]string{"pair programming", "codebase Q&A"}),
			TargetUsers:     ptr([]string{"software engineers"}),
			KeyFeatures:     ptr([]string{"tab completion", "agent mode"}),
			TechStack:       ptr([]string{"Electron", "TypeScript"}),
			PricingModel:    ptr("freemium"),
			Maturity:        ptr("GA"),
		},
		Vocabulary: client.Vocabulary{
			UserTerms:          ptr([]string{"cursor", "cursor ai"}),
			FeatureJargon:      ptr([]string{"composer", "cmd-k", "tab model"}),
			CommonMisspellings: ptr([]string{"curser"}),
			AdjacentProducts:   ptr([]string{"Copilot", "Windsurf"}),
		},
		Disambiguation: client.Disambiguation{
			AmbiguityScore: 0.75,
			NameCollisions: ptr([]client.NameCollision{{
				Name:          "database cursor",
				WhatItIs:      "a control structure for traversing query results",
				WhyConfusable: ptr("identical word, heavy usage in developer forums"),
				EvidenceUrl:   "https://en.wikipedia.org/wiki/Cursor_(databases)",
			}}),
			PositiveSignals: ptr([]string{"cursor.com", "composer"}),
			NegativeSignals: ptr([]string{"SQL", "FETCH NEXT"}),
		},
		Provenance: client.Provenance{
			GeneratedAt:     time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC),
			RuntimeMs:       41230,
			Sources:         ptr([]client.Source{{Url: "https://cursor.com", Via: "firecrawl", FetchedAt: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)}}),
			FieldConfidence: ptr(map[string]float32{"what.license": 0.2, "what.category": 0.95}),
			DegradedSources: ptr([]string{"dropped unsourced collision 'Cursor Bank'"}),
		},
	}
}

func TestMarkdownMatchesGolden(t *testing.T) {
	got := Markdown(fixture())
	golden := filepath.Join("testdata", "product.md")

	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run: go test ./internal/render -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("markdown drifted from golden.\n--- got ---\n%s", got)
	}
}

func TestMarkdownIsDeterministic(t *testing.T) {
	if Markdown(fixture()) != Markdown(fixture()) {
		t.Error("Markdown is not deterministic across runs")
	}
}

func TestMarkdownSurfacesCollisionsAndLowConfidence(t *testing.T) {
	got := Markdown(fixture())

	for _, want := range []string{
		"database cursor",
		"https://en.wikipedia.org/wiki/Cursor_(databases)",
		"Ambiguity score: 0.75",
		"what.license (0.20)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q from rendered markdown", want)
		}
	}
	if strings.Contains(got, "what.category") {
		t.Error("high-confidence fields should not be listed as low-confidence")
	}
}

func TestMarkdownHandlesAnEmptyDossier(t *testing.T) {
	got := Markdown(client.ProductDossier{
		Identity: client.Identity{CanonicalName: "Unknown", Slug: "unknown"},
	})

	if !strings.Contains(got, "No description could be established") {
		t.Error("empty dossier should say so rather than render blanks")
	}
	if !strings.Contains(got, "No name collisions were observed") {
		t.Error("empty collision list should be stated explicitly")
	}
}

func TestAmbiguityVerdictBands(t *testing.T) {
	cases := map[float32]string{0.0: "uncontested", 0.2: "owns most", 0.5: "meaningful share", 0.9: "crowded"}
	for score, want := range cases {
		if got := AmbiguityVerdict(score); !strings.Contains(got, want) {
			t.Errorf("AmbiguityVerdict(%.1f) = %q, want it to mention %q", score, got, want)
		}
	}
}
