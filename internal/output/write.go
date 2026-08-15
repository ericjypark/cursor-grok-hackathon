// Package output persists a run's results for review and for downstream stages.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/render"
)

// Paths reports where a run's artifacts landed.
type Paths struct {
	JSON  string // product.json — the dossier T2 consumes
	MD    string // product.md   — the same, for a human to sanity-check
	Posts string // posts.json   — T1's scraped posts; empty when T1 did not run
}

// Write emits the dossier as product.json and product.md, plus posts.json when
// the run got as far as scraping. The three files are written separately rather
// than as one blob because each has a different reader: T2 reads posts.json, a
// human reads product.md, and T1 reads product.json.
func Write(baseDir, slug string, note client.FieldNote) (Paths, error) {
	var out Paths
	dir := filepath.Join(baseDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return out, fmt.Errorf("cannot create %s: %w", dir, err)
	}

	blob, err := json.MarshalIndent(note.Dossier, "", "  ")
	if err != nil {
		return out, fmt.Errorf("cannot encode dossier: %w", err)
	}
	out.JSON = filepath.Join(dir, "product.json")
	if err = os.WriteFile(out.JSON, append(blob, '\n'), 0o644); err != nil {
		return out, err
	}

	out.MD = filepath.Join(dir, "product.md")
	if err = os.WriteFile(out.MD, []byte(render.Markdown(note.Dossier)), 0o644); err != nil {
		return out, err
	}

	if note.Harvest == nil {
		return out, nil
	}
	posts, err := json.MarshalIndent(note.Harvest, "", "  ")
	if err != nil {
		return out, fmt.Errorf("cannot encode harvest: %w", err)
	}
	out.Posts = filepath.Join(dir, "posts.json")
	if err = os.WriteFile(out.Posts, append(posts, '\n'), 0o644); err != nil {
		return out, err
	}
	return out, nil
}
