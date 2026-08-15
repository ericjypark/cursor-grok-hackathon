// Package render turns a dossier into the human-readable review document.
//
// Rendering lives in the CLI, not the backend: the API returns structured
// data and presentation belongs at the edge.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func list(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

// AmbiguityVerdict states, in words, what the score means for downstream
// scraping. The number alone doesn't tell a reader whether to worry.
func AmbiguityVerdict(score float32) string {
	switch {
	case score >= 0.7:
		return "crowded, so downstream scrapers must filter hard"
	case score >= 0.4:
		return "mixed, so expect a meaningful share of unrelated hits"
	case score > 0:
		return "clean — the product owns most of its name space"
	default:
		return "uncontested — no rival use of the name was observed"
	}
}

func bullets(sb *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "**%s**\n\n", heading)
	for _, it := range items {
		fmt.Fprintf(sb, "- %s\n", it)
	}
	sb.WriteString("\n")
}

func inline(sb *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(sb, "**%s** — %s\n\n", heading, strings.Join(items, ", "))
}

// Markdown renders the dossier. Deterministic: same input, same bytes.
func Markdown(d client.ProductDossier) string {
	var sb strings.Builder
	id, what, vocab, dis, prov := d.Identity, d.What, d.Vocabulary, d.Disambiguation, d.Provenance

	fmt.Fprintf(&sb, "# %s\n\n", id.CanonicalName)
	if t := str(id.Tagline); t != "" {
		fmt.Fprintf(&sb, "> %s\n\n", t)
	}

	facts := [][2]string{
		{"Homepage", str(id.Homepage)},
		{"Repository", str(id.Repo)},
		{"Docs", str(id.DocsUrl)},
		{"Owner", str(id.OrgOrOwner)},
		{"Category", str(what.Category)},
		{"Maturity", str(what.Maturity)},
		{"Pricing", str(what.PricingModel)},
		{"License", str(what.License)},
	}
	if pkgs := id.PackageNames; pkgs != nil && len(*pkgs) > 0 {
		parts := make([]string, 0, len(*pkgs))
		for _, p := range *pkgs {
			parts = append(parts, fmt.Sprintf("%s: `%s`", p.Registry, p.Name))
		}
		facts = append(facts, [2]string{"Packages", strings.Join(parts, ", ")})
	}
	if cmds := list(id.CliCommands); len(cmds) > 0 {
		facts = append(facts, [2]string{"CLI", "`" + strings.Join(cmds, "`, `") + "`"})
	}
	if aliases := list(id.Aliases); len(aliases) > 0 {
		facts = append(facts, [2]string{"Aliases", strings.Join(aliases, ", ")})
	}

	sb.WriteString("| | |\n|---|---|\n")
	for _, f := range facts {
		if f[1] != "" {
			fmt.Fprintf(&sb, "| %s | %s |\n", f[0], f[1])
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## What it is\n\n")
	if desc := str(what.Description); desc != "" {
		sb.WriteString(desc + "\n\n")
	} else {
		sb.WriteString("_No description could be established from the evidence._\n\n")
	}
	bullets(&sb, "Primary use cases", list(what.PrimaryUseCases))
	bullets(&sb, "Target users", list(what.TargetUsers))
	bullets(&sb, "Key features", list(what.KeyFeatures))
	inline(&sb, "Tech stack", list(what.TechStack))

	sb.WriteString("## Vocabulary users use\n\n")
	inline(&sb, "Terms", list(vocab.UserTerms))
	inline(&sb, "Distinctive jargon", list(vocab.FeatureJargon))
	inline(&sb, "Common misspellings", list(vocab.CommonMisspellings))
	inline(&sb, "Adjacent products", list(vocab.AdjacentProducts))

	fmt.Fprintf(&sb, "## Disambiguation\n\n**Ambiguity score: %.2f** — %s\n\n",
		dis.AmbiguityScore, AmbiguityVerdict(dis.AmbiguityScore))

	if cols := dis.NameCollisions; cols != nil && len(*cols) > 0 {
		sb.WriteString("### Other things sharing this name\n\n")
		sb.WriteString("| Name | What it is | Why confusable | Evidence |\n|---|---|---|---|\n")
		for _, c := range *cols {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
				c.Name, c.WhatItIs, str(c.WhyConfusable), c.EvidenceUrl)
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("_No name collisions were observed in the evidence._\n\n")
	}
	inline(&sb, "Positive signals", list(dis.PositiveSignals))
	inline(&sb, "Negative signals", list(dis.NegativeSignals))
	if n := str(dis.Notes); n != "" {
		fmt.Fprintf(&sb, "%s\n\n", n)
	}

	sb.WriteString("## Provenance\n\n")
	n := len(sources(prov.Sources))
	fmt.Fprintf(&sb, "Generated %s in %dms from %d %s.\n\n",
		prov.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		prov.RuntimeMs, n, plural(n, "source", "sources"))

	if fc := prov.FieldConfidence; fc != nil && len(*fc) > 0 {
		var low []string
		for k, v := range *fc {
			if v < 0.5 {
				low = append(low, fmt.Sprintf("%s (%.2f)", k, v))
			}
		}
		sort.Strings(low)
		inline(&sb, "Low-confidence fields", low)
	}
	bullets(&sb, "Degraded or dropped", list(prov.DegradedSources))

	return sb.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func sources(p *[]client.Source) []client.Source {
	if p == nil {
		return nil
	}
	return *p
}
