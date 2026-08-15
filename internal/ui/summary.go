package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/render"
)

// keyCol is the width of the left-hand label gutter every summary row hangs off.
const keyCol = 12

// Summary renders the post-run readout: the headline, the disambiguation
// verdict, and where the artifacts landed.
func Summary(d client.ProductDossier, jsonPath, mdPath string) string {
	var sb strings.Builder
	w := frameWidth

	head := Gradient(spaced(d.Identity.CanonicalName), true)
	if cat := strings.TrimSpace(deref(d.What.Category)); cat != "" {
		head += "  " + Dim.Render("·") + "  " + Muted.Render(cat)
	}
	sb.WriteString("\n  " + head + "\n")
	sb.WriteString("  " + Rule(w) + "\n\n")

	score := d.Disambiguation.AmbiguityScore
	verdict := Good
	if score >= 0.4 {
		verdict = Warn
	}
	if score >= 0.7 {
		verdict = Bad
	}
	sb.WriteString(row("AMBIGUITY",
		Meter(barWidth, float64(score))+"  "+Title.Render(fmt.Sprintf("%.2f", score))))
	sb.WriteString(row("", verdict.Render(render.AmbiguityVerdict(score))))

	collisions := d.Disambiguation.NameCollisions
	n := 0
	if collisions != nil {
		n = len(*collisions)
	}
	sb.WriteString("\n" + row("COLLISIONS", Title.Render(fmt.Sprintf("%d", n))+"  "+
		Dim.Render(plural(n, "other thing shares this name", "other things share this name"))))
	if collisions != nil {
		for _, c := range *collisions {
			sb.WriteString(row("", Accent.Render("›")+" "+Body.Render(c.Name)+"  "+Dim.Render(c.WhatItIs)))
		}
	}

	if terms := d.Vocabulary.FeatureJargon; terms != nil && len(*terms) > 0 {
		// Styled separately then joined: nesting a rendered string inside
		// another Render leaves everything after the inner reset unstyled.
		parts := make([]string, 0, len(*terms))
		for _, t := range *terms {
			parts = append(parts, Body.Render(t))
		}
		sb.WriteString("\n" + row("TERMS", strings.Join(parts, Dim.Render(", "))))
	}

	if deg := d.Provenance.DegradedSources; deg != nil && len(*deg) > 0 {
		sb.WriteString("\n  " + Warn.Render(fmt.Sprintf("▲ %d %s degraded or dropped — see product.md",
			len(*deg), plural(len(*deg), "source", "sources"))))
		sb.WriteString("\n")
	}

	sb.WriteString("\n  " + Rule(w) + "\n")
	sb.WriteString("  " + Gradient("→", false) + " " + Body.Render(mdPath) + "\n")
	sb.WriteString("    " + Dim.Render(jsonPath) + "\n")
	return sb.String()
}

// row lays out one label/value line, wrapping the value under an empty gutter
// when the caller passes no key.
func row(key, value string) string {
	label := ""
	if key != "" {
		label = Dim.Render(key)
	}
	pad := keyCol - lipgloss.Width(key)
	if pad < 1 {
		pad = 1
	}
	return "  " + label + strings.Repeat(" ", pad) + value + "\n"
}

// spaced letter-spaces a wordmark, which is what makes the gradient read as a
// gradient rather than as one oddly-tinted word.
func spaced(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "UNKNOWN"
	}
	return strings.Join(strings.Split(s, ""), " ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
