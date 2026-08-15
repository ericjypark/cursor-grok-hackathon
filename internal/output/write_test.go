package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

func fixture() client.FieldNote {
	return client.FieldNote{Dossier: client.ProductDossier{
		Identity:       client.Identity{CanonicalName: "Acme", Slug: "acme"},
		Disambiguation: client.Disambiguation{AmbiguityScore: 0.5},
		Provenance:     client.Provenance{GeneratedAt: time.Now().UTC(), RuntimeMs: 7},
	}}
}

// withPosts is a run that got as far as scraping.
func withPosts() client.FieldNote {
	n := fixture()
	n.Harvest = &client.Harvest{
		Live:       true,
		SourceNote: "live scrape",
		Posts: &[]client.Post{{
			Id: "post_rd_abc", Source: "reddit", Url: "https://reddit.com/r/acme/comments/1/",
		}},
	}
	return n
}

func TestWriteEmitsBothArtifacts(t *testing.T) {
	dir := t.TempDir()

	paths, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if want := filepath.Join(dir, "acme", "product.json"); paths.JSON != want {
		t.Errorf("json path = %q, want %q", paths.JSON, want)
	}
	for _, p := range []string{paths.JSON, paths.MD} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was not written: %v", p, err)
		}
	}
	if paths.Posts != "" {
		t.Errorf("a t0-only run wrote posts.json at %q", paths.Posts)
	}
}

func TestScrapedPostsAreWrittenForT2(t *testing.T) {
	dir := t.TempDir()

	paths, err := Write(dir, "acme", withPosts())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if want := filepath.Join(dir, "acme", "posts.json"); paths.Posts != want {
		t.Fatalf("posts path = %q, want %q", paths.Posts, want)
	}

	blob, err := os.ReadFile(paths.Posts)
	if err != nil {
		t.Fatal(err)
	}
	var back client.Harvest
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("T2 could not parse posts.json: %v", err)
	}
	if back.Posts == nil || len(*back.Posts) != 1 {
		t.Errorf("round-trip lost the posts: %+v", back.Posts)
	}
	// Whether the posts are real or recorded has to survive to disk, or a
	// downstream reader cannot tell one from the other.
	if !back.Live {
		t.Error("round-trip lost the live flag")
	}
}

func TestWrittenJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	paths, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatal(err)
	}

	blob, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	var back client.ProductDossier
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("downstream stages could not parse product.json: %v", err)
	}
	if back.Identity.CanonicalName != "Acme" {
		t.Errorf("round-trip lost data: %+v", back.Identity)
	}
}

func TestWriteIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "acme", fixture()); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(dir, "acme", fixture()); err != nil {
		t.Fatalf("second write into an existing directory failed: %v", err)
	}
}

func TestWrittenMarkdownIsHumanReadable(t *testing.T) {
	dir := t.TempDir()
	paths, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatal(err)
	}

	blob, err := os.ReadFile(paths.MD)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(blob), "# Acme") {
		t.Errorf("markdown should lead with the product name, got %.40q", blob)
	}
}
