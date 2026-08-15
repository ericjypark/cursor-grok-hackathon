// Package output persists the dossier for review and for downstream stages.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
	"github.com/ericjypark/cursor-grok-hackathon/internal/render"
)

// Write emits product.json (consumed by T1/T2) and product.md (for a human to
// sanity-check before any scrape budget is spent).
func Write(baseDir, slug string, d client.ProductDossier) (jsonPath, mdPath string, err error) {
	dir := filepath.Join(baseDir, slug)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("cannot create %s: %w", dir, err)
	}

	blob, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("cannot encode dossier: %w", err)
	}
	jsonPath = filepath.Join(dir, "product.json")
	if err = os.WriteFile(jsonPath, append(blob, '\n'), 0o644); err != nil {
		return "", "", err
	}

	mdPath = filepath.Join(dir, "product.md")
	if err = os.WriteFile(mdPath, []byte(render.Markdown(d)), 0o644); err != nil {
		return "", "", err
	}
	return jsonPath, mdPath, nil
}
