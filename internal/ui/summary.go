package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/output"
	"github.com/ericjypark/cursor-grok-hackathon/internal/render"
)

// keyCol is the width of the left-hand label gutter every summary row hangs off.
const keyCol = 12

// Summary renders the post-run readout: the headline, the disambiguation
// verdict, what the scrape found, and where the artifacts landed.
func Summary(note client.FieldNote, paths output.Paths) string {
	d := note.Dossier
	var sb strings.Builder
	cols, _ := TermSize()
	w := Fit(cols)

	cat := strings.TrimSpace(deref(d.What.Category))
	if cat != "" {
		cat = Muted.Render(cat)
	}
	// The wordmark is letter-spaced, so it is twice as wide as the name reads.
	// Clip it against what the category column leaves, or a long canonical name
	// wraps the headline and pushes the rule out of alignment.
	head := Gradient(clip(spaced(d.Identity.CanonicalName), w-lipgloss.Width(cat)-2), true)
	sb.WriteString("\n  " + Spread(w, head, cat) + "\n")
	sb.WriteString("  " + Rule(w) + "\n")
	if tag := strings.TrimSpace(deref(d.Identity.Tagline)); tag != "" {
		sb.WriteString("  " + Muted.Render(tag) + "\n")
	}
	sb.WriteString("\n")

	score := d.Disambiguation.AmbiguityScore
	verdict := Good
	if score >= 0.4 {
		verdict = Warn
	}
	if score >= 0.7 {
		verdict = Bad
	}
	sb.WriteString(rowW(w, "AMBIGUITY",
		Meter(min(meterWidth(w), 18), float64(score))+"  "+Title.Render(fmt.Sprintf("%.2f", score)),
		verdict.Render(render.AmbiguityVerdict(score))))

	collisions := d.Disambiguation.NameCollisions
	n := 0
	if collisions != nil {
		n = len(*collisions)
	}
	sb.WriteString("\n" + row("COLLISIONS", Title.Render(fmt.Sprintf("%d", n))+"  "+
		Dim.Render(plural(n, "other thing shares this name", "other things share this name"))))
	if collisions != nil {
		for _, c := range *collisions {
			name := Accent.Render("›") + " " + Body.Render(c.Name)
			// One row per collision. A gloss that half-wraps under the gutter
			// has no visual subordination, so it reads as another entry.
			sb.WriteString(rowW(w, "", name,
				Dim.Render(clip(c.WhatItIs, rightBudget(w, lipgloss.Width(name))))))
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

	sb.WriteString(harvestBlock(note.Harvest, w))

	if deg := d.Provenance.DegradedSources; deg != nil && len(*deg) > 0 {
		sb.WriteString("\n  " + Warn.Render(fmt.Sprintf("▲ %d %s degraded or dropped — see product.md",
			len(*deg), plural(len(*deg), "source", "sources"))))
		sb.WriteString("\n")
	}

	sb.WriteString("\n  " + Rule(w) + "\n")
	sb.WriteString("  " + Spread(w,
		Gradient("→", false)+" "+Body.Render(paths.MD), Dim.Render("markdown")) + "\n")
	sb.WriteString("  " + Spread(w,
		Dim.Render("  "+paths.JSON), Dim.Render("json")) + "\n")
	if paths.Posts != "" {
		sb.WriteString("  " + Spread(w,
			Dim.Render("  "+paths.Posts), Dim.Render("posts")) + "\n")
	}
	return sb.String()
}

// topPosts is how many scraped discussions the readout lists. Enough to prove
// the scrape worked and to make the loudest complaints visible; short enough
// that the summary still fits one screen.
const topPosts = 5

// harvestBlock renders T1's result: where it looked, what it found, and — when
// the live scraper was unavailable — that the posts are recorded ones.
func harvestBlock(h *client.Harvest, w int) string {
	if h == nil {
		return ""
	}
	var sb strings.Builder

	targets := []string{}
	if r := h.Targets.Reddit; r != nil {
		if subs := r.Subreddits; subs != nil {
			for _, name := range *subs {
				targets = append(targets, Body.Render("r/"+name))
			}
		}
		if q := r.SearchQueries; q != nil {
			for _, query := range *q {
				targets = append(targets, Muted.Render("\u201c"+query+"\u201d"))
			}
		}
	}
	if len(targets) > 0 {
		sb.WriteString("\n" + row("SOURCES", strings.Join(targets, Dim.Render(", "))))
	}

	posts := []client.Post{}
	if h.Posts != nil {
		posts = *h.Posts
	}
	label := Good.Render("live")
	if !h.Live {
		label = Warn.Render("recorded")
	}
	sb.WriteString("\n" + rowW(w, "POSTS",
		Title.Render(fmt.Sprintf("%d", len(posts)))+"  "+
			Dim.Render(plural(len(posts), "discussion scraped", "discussions scraped")),
		label))

	// Loudest first: engagement is the closest thing to a severity signal we
	// have before T2 reads the threads.
	ranked := append([]client.Post{}, posts...)
	sort.SliceStable(ranked, func(i, j int) bool { return score(ranked[i]) > score(ranked[j]) })
	if len(ranked) > topPosts {
		ranked = ranked[:topPosts]
	}
	for _, p := range ranked {
		meta := Dim.Render(fmt.Sprintf("%s  %d", deref(p.Channel), score(p)))
		// The title's budget is whatever the frame has left once the gutter and
		// the channel column are spoken for, so a wide terminal shows more of
		// the title instead of the same fixed span.
		sb.WriteString(rowW(w, "", Accent.Render("\u203a")+" "+
			Body.Render(clip(headline(p), rightBudget(w, lipgloss.Width(meta))-2)), meta))
	}
	if n := len(posts) - len(ranked); n > 0 {
		sb.WriteString(row("", Dim.Render(fmt.Sprintf("+ %d more in posts.json", n))))
	}

	// A fallback run substituted recorded signals about a different product. A
	// dim word at the right edge is not enough to stop a reader taking these
	// for their own users, so the source note gets a warning of its own.
	if !h.Live {
		if n := strings.Join(strings.Fields(h.SourceNote), " "); n != "" {
			sb.WriteString("\n  " + Warn.Render("\u25b2 "+clip(n, w-2)) + "\n")
		}
	}
	return sb.String()
}

func score(p client.Post) int {
	if p.Score == nil {
		return 0
	}
	return *p.Score
}

// headline is a post's title, or the opening of its body when it has none —
// X posts have no title at all, and Reddit link posts have no body.
func headline(p client.Post) string {
	text := strings.TrimSpace(deref(p.Title))
	if text == "" {
		text = strings.TrimSpace(deref(p.Body))
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return p.Url
	}
	return text
}

// clip trims text to a display-width budget. Everything else in this file
// measures in terminal cells, and so must this: a byte-length cut stops a
// Korean title a third of the way in and can split a rune in half.
func clip(text string, budget int) string {
	if budget < 4 {
		budget = 4
	}
	if lipgloss.Width(text) <= budget {
		return text
	}
	return strings.TrimRight(ansi.Truncate(text, budget-1, ""), " ") + "\u2026"
}

// rightBudget is the room a two-column row leaves its second column once the
// gutter and the first column are spoken for \u2014 the same arithmetic rowW uses
// to decide whether the two fit, so a value clipped to it always does.
func rightBudget(width, leftW int) int {
	return width - keyCol - leftW - 2
}

// rowW is row with a right-hand column pinned to the frame's right edge. When
// the two columns cannot share a line the right one drops to its own row under
// the gutter rather than being truncated away.
func rowW(width int, key, value, right string) string {
	line := strings.TrimSuffix(row(key, value), "\n")
	if right == "" {
		return line + "\n"
	}
	// line carries the two-space rail that sits outside the frame, so it is
	// measured against width+2 — the same span the fitting branch spreads over.
	if lipgloss.Width(line)+lipgloss.Width(right)+2 > width+2 {
		return line + "\n" + row("", right)
	}
	return Spread(width+2, line, right) + "\n"
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
