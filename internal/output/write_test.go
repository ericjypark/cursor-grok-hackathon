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

func fixture() client.ProductDossier {
	return client.ProductDossier{
		Identity:       client.Identity{CanonicalName: "Acme", Slug: "acme"},
		Disambiguation: client.Disambiguation{AmbiguityScore: 0.5},
		Provenance:     client.Provenance{GeneratedAt: time.Now().UTC(), RuntimeMs: 7},
	}
}

func TestWriteEmitsBothArtifacts(t *testing.T) {
	dir := t.TempDir()

	jsonPath, mdPath, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if want := filepath.Join(dir, "acme", "product.json"); jsonPath != want {
		t.Errorf("json path = %q, want %q", jsonPath, want)
	}
	for _, p := range []string{jsonPath, mdPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was not written: %v", p, err)
		}
	}
}

func TestWrittenJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	jsonPath, _, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatal(err)
	}

	blob, err := os.ReadFile(jsonPath)
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
	if _, _, err := Write(dir, "acme", fixture()); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Write(dir, "acme", fixture()); err != nil {
		t.Fatalf("second write into an existing directory failed: %v", err)
	}
}

func TestWrittenMarkdownIsHumanReadable(t *testing.T) {
	dir := t.TempDir()
	_, mdPath, err := Write(dir, "acme", fixture())
	if err != nil {
		t.Fatal(err)
	}

	blob, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(blob), "# Acme") {
		t.Errorf("markdown should lead with the product name, got %.40q", blob)
	}
}
