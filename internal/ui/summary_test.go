package ui

import (
	"strings"
	"testing"

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
	got := Summary(note(nil), paths())
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
	}), paths())

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
	}), paths())

	if !strings.Contains(got, "recorded") {
		t.Errorf("fallback posts are not flagged:\n%s", got)
	}
}

func TestSummaryFallsBackToTheBodyWhenAPostHasNoTitle(t *testing.T) {
	body := "third month in a row acme billed me after I cancelled"
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Posts: &[]client.Post{{Id: "x", Source: "x", Url: "https://x.com/a/1", Body: &body}},
	}), paths())

	if !strings.Contains(got, "third month in a row") {
		t.Errorf("an X post with no title rendered nothing:\n%s", got)
	}
}

func TestSummaryTruncatesRatherThanWrapping(t *testing.T) {
	long := strings.Repeat("a very long complaint ", 20)
	got := Summary(note(&client.Harvest{
		Live: true, SourceNote: "live scrape",
		Posts: &[]client.Post{post(long, 5)},
	}), paths())

	for _, line := range strings.Split(got, "\n") {
		if len(line) > 400 {
			t.Errorf("a long title was not truncated: %d chars", len(line))
		}
	}
}

func TestSummaryReportsPostsPathWhenThereIsOne(t *testing.T) {
	p := paths()
	p.Posts = "out/acme/posts.json"
	got := Summary(note(&client.Harvest{Live: true, SourceNote: "live scrape"}), p)
	if !strings.Contains(got, "posts.json") {
		t.Error("the posts artifact is not reported")
	}
}
