package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/output"
)

func note(h *client.Harvest) client.FieldNote {
	return client.FieldNote{
		Dossier: client.ProductDossier{
			Identity:       client.Identity{CanonicalName: "Acme", Slug: "acme"},
			Disambiguation: client.Disambiguation{AmbiguityScore: 0.5},
		},
		Harvest: h,
	}
}

func post(title string, score int) client.Post {
	return client.Post{Id: "x", Source: "reddit", Url: "https://r/acme", Title: &title, Score: &score}
}

func paths() output.Paths {
	return output.Paths{JSON: "out/acme/product.json", MD: "out/acme/product.md"}
}

func TestSummaryOmitsTheScrapeBlockWhenT1DidNotRun(t *testing.T) {
	got := Summary(note(nil), paths(), 0)
	if strings.Contains(got, "POSTS") {
		t.Error("a t0-only run should not claim a post count")
	}
}

func TestSummaryListsTheLoudestPostsFirst(t *testing.T) {
	subs := []string{"acme"}
	got := Summary(note(&client.Harvest{
		Live:       true,
		SourceNote: "live scrape",
		Targets:    client.ScrapeTargets{Reddit: &client.RedditTargets{Subreddits: &subs}},
		Posts:      &[]client.Post{post("quiet gripe", 3), post("everyone is furious", 900)},
	}), paths(), 0)

	if !strings.Contains(got, "r/acme") {
		t.Error("the subreddits T1 chose should be visible")
	}
	loud, quiet := strings.Index(got, "everyone is furious"), strings.Index(got, "quiet gripe")
	if loud < 0 || quiet < 0 {
		t.Fatalf("posts missing from the readout:\n%s", got)
	}
	if loud > quiet {
		t.Error("posts are not ranked by engagement")
	}
}

// Recorded signals are about a different product. A readout that does not say
// so invites the reader to believe they are looking at their own users.
func TestSummaryFlagsRecordedPosts(t *testing.T) {
	got := Summary(note(&client.Harvest{
		Live:       false,
		SourceNote: "recorded signals — the live scraper was unavailable",
		Posts:      &[]client.Post{post("a complaint", 10)},
	}), paths(), 0)

	if !strings.Contains(got, "recorded") {
		t.Errorf("fallback posts are not flagged:\n%s", got)
	}
}

func TestSummaryFallsBackToTheBodyWhenAPostHasNoTitle(t *testing.T) {
	body := "third month in a row acme billed me after I cancelled"
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Posts: &[]client.Post{{Id: "x", Source: "x", Url: "https://x.com/a/1", Body: &body}},
	}), paths(), 0)

	if !strings.Contains(got, "third month in a row") {
		t.Errorf("an X post with no title rendered nothing:\n%s", got)
	}
}

func TestSummaryTruncatesRatherThanWrapping(t *testing.T) {
	long := strings.Repeat("a very long complaint ", 20)
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Posts: &[]client.Post{post(long, 5)},
	}), paths(), 0)

	for _, line := range strings.Split(got, "\n") {
		if len(line) > 400 {
			t.Errorf("a long title was not truncated: %d chars", len(line))
		}
	}
}

func TestSummaryReportsPostsPathWhenThereIsOne(t *testing.T) {
	p := paths()
	p.Posts = "out/acme/posts.json"
	got := Summary(note(&client.Harvest{Live: true, SourceNote: "live scrape"}), p, 0)
	if !strings.Contains(got, "posts.json") {
		t.Error("the posts artifact is not reported")
	}
}

// A byte-length cut stops a Korean title a third of the way in, and can land
// mid-rune and emit a replacement character.
func TestClipMeasuresDisplayWidthNotBytes(t *testing.T) {
	ko := "커서 에디터가 자꾸 파일을 전부 다시 쓰는 이유가 뭔가요 정말 궁금합니다 도와주세요"
	got := clip(ko, 40)
	if w := lipgloss.Width(got); w > 40 {
		t.Errorf("clipped to %d cells, budget was 40: %q", w, got)
	}
	if w := lipgloss.Width(got); w < 36 {
		t.Errorf("clipped to %d cells, far short of the 40 available: %q", w, got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("the cut split a rune: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a clipped string should end in an ellipsis: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("clip produced invalid utf-8: %q", got)
	}
}

func TestClipLeavesTextThatAlreadyFits(t *testing.T) {
	if got := clip("short enough", 40); got != "short enough" {
		t.Errorf("clip trimmed a string that fits: %q", got)
	}
}

// The byte-slice version cut this exactly mid-rune.
func TestClipDoesNotSplitARune(t *testing.T) {
	for budget := 4; budget < 30; budget++ {
		got := clip("한글이 섞인 제목 with ascii tail", budget)
		if strings.ContainsRune(got, utf8.RuneError) || !utf8.ValidString(got) {
			t.Errorf("budget %d split a rune: %q", budget, got)
		}
		if w := lipgloss.Width(got); w > budget {
			t.Errorf("budget %d overflowed to %d: %q", budget, w, got)
		}
	}
}

func TestSummaryKeepsACJKTitleOnOneRow(t *testing.T) {
	ko := "커서 에디터가 자꾸 파일을 전부 다시 쓰는 이유가 뭔가요 정말 궁금합니다 도와주세요"
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Posts: &[]client.Post{post(ko, 5)},
	}), paths(), 0)

	if !strings.Contains(got, "커서 에디터가") {
		t.Fatalf("the Korean title rendered nothing:\n%s", got)
	}
	w := Fit(termCols())
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > w+2 {
			t.Errorf("line overflows the frame (%d > %d): %q", lipgloss.Width(line), w+2, line)
		}
	}
}

// Recorded posts are about a different product entirely. The dim "recorded"
// tag at the right edge is easy to miss; the source note must not be.
func TestSummarySurfacesTheSourceNoteWhenPostsAreRecorded(t *testing.T) {
	got := Summary(note(&client.Harvest{
		Live:       false,
		SourceNote: "recorded signals — these are about Perplexity, not this product",
		Posts:      &[]client.Post{post("a complaint", 10)},
	}), paths(), 0)

	if !strings.Contains(got, "these are about Perplexity, not this product") {
		t.Errorf("the source note is not surfaced on a fallback run:\n%s", got)
	}
	if !strings.Contains(got, "▲ recorded signals") {
		t.Errorf("the source note is not carrying the warning marker:\n%s", got)
	}
}

func TestSummaryDoesNotWarnWhenTheScrapeWasLive(t *testing.T) {
	got := Summary(note(&client.Harvest{
		Live:       true,
		SourceNote: "live scrape of r/acme",
		Posts:      &[]client.Post{post("a complaint", 10)},
	}), paths(), 0)

	if strings.Contains(got, "▲ live scrape") {
		t.Errorf("a live run should not raise a source warning:\n%s", got)
	}
}

// A gloss that half-wraps under the gutter reads as a separate collision.
func TestSummaryKeepsEachCollisionOnOneRow(t *testing.T) {
	n := note(nil)
	n.Dossier.Disambiguation.NameCollisions = &[]client.NameCollision{
		{Name: "database cursor", WhatItIs: "control structure for traversing query result sets one row at a time", EvidenceUrl: "https://example.com/1"},
		{Name: "Acme Corp", WhatItIs: "short", EvidenceUrl: "https://example.com/2"},
	}
	got := Summary(n, paths(), 0)

	rows := 0
	w := Fit(termCols())
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "›") {
			rows++
		}
		if lipgloss.Width(line) > w+2 {
			t.Errorf("collision row overflows the frame: %q", line)
		}
		// Every collision line must carry its own bullet — a continuation line
		// holding only the gloss is the bug this guards.
		if strings.Contains(line, "control structure") && !strings.Contains(line, "›") {
			t.Errorf("a collision gloss wrapped onto its own line: %q", line)
		}
	}
	if rows != 2 {
		t.Errorf("expected one row per collision, got %d:\n%s", rows, got)
	}
}

// A letter-spaced wordmark is twice as wide as the name reads, so a long
// canonical name would wrap the headline and misalign the rule under it.
func TestSummaryClipsALongCanonicalName(t *testing.T) {
	n := note(nil)
	n.Dossier.Identity.CanonicalName = strings.Repeat("supercalifragilistic", 6)
	got := Summary(n, paths(), 0)

	w := Fit(termCols())
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > w+2 {
			t.Errorf("the wordmark overflowed the frame (%d > %d): %q",
				lipgloss.Width(line), w+2, line)
		}
	}
}

func termCols() int {
	c, _ := TermSize()
	return c
}

// The SOURCES row was the last one in the readout that could still wrap: it
// joined every subreddit and every search query with no budget at all, and a
// dozen of them ran straight off an 80-column frame.
func TestSummarySourcesRowStaysInsideTheFrame(t *testing.T) {
	subs := []string{"cursor", "ChatGPTCoding", "LocalLLaMA", "programming", "webdev", "javascript"}
	queries := []string{"cursor composer is slow", "cursor billing charged twice",
		"cursorrules not respected", "cursor tab model regression"}
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Targets: client.ScrapeTargets{Reddit: &client.RedditTargets{
			Subreddits: &subs, SearchQueries: &queries}},
	}), paths(), 0)

	w := Fit(termCols())
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > w+2 {
			t.Errorf("a row overflows the frame (%d > %d): %q", lipgloss.Width(line), w+2, line)
		}
	}
	// What it drops it has to count, or the readout quietly understates where
	// T1 actually looked.
	if !strings.Contains(got, "more") {
		t.Errorf("the sources it could not fit went unreported:\n%s", got)
	}
	if !strings.Contains(got, "r/cursor") {
		t.Errorf("the sources row rendered nothing at all:\n%s", got)
	}
}

// Every fragment has to fit, at any width — including one so narrow that only
// the tally survives.
func TestJoinToFitNeverOverflowsItsBudget(t *testing.T) {
	frags := []frag{{"r/cursor", Body}, {"r/ChatGPTCoding", Body},
		{"“a fairly long search query about billing”", Muted}, {"r/webdev", Body}}
	for budget := 10; budget < 90; budget++ {
		if w := lipgloss.Width(joinToFit(frags, budget)); w > budget {
			t.Errorf("budget %d overflowed to %d", budget, w)
		}
	}
}

// The wall clock the progress view was counting is the proof the minutes
// bought something, so it has to survive the TUI's teardown.
func TestSummaryReportsTheRunClock(t *testing.T) {
	got := Summary(note(nil), paths(), 254*time.Second)
	if !strings.Contains(got, "04:14") {
		t.Errorf("the run's elapsed time is missing:\n%s", got)
	}
	// A run with no measured duration says nothing rather than claiming zero.
	if strings.Contains(Summary(note(nil), paths(), 0), "elapsed") {
		t.Error("a zero duration still drew a clock")
	}
}

// The score is the most interesting number the dossier holds; the meter is
// corroboration. A readout that leads with the bar reads as a progress bar.
func TestSummaryLeadsTheAmbiguityRowWithTheNumber(t *testing.T) {
	n := note(nil)
	n.Dossier.Disambiguation.AmbiguityScore = 0.75
	for _, line := range strings.Split(Summary(n, paths(), 0), "\n") {
		if !strings.Contains(line, "AMBIGUITY") {
			continue
		}
		if score, bar := strings.Index(line, "0.75"), strings.Index(line, "▰"); score < 0 || bar < score {
			t.Errorf("the ambiguity row does not lead with its score: %q", line)
		}
		return
	}
	t.Error("no ambiguity row was rendered")
}
