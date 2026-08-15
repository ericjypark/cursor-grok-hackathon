# fieldnote

CLI frontend for **T0 — product understanding**, the first stage of the
field-note pipeline.

You give it a product website (plus, optionally, a GitHub repo and a free-text
description). It calls the [field-note backend](https://github.com/in-sol-ence/field-note-backend),
streams live progress, and writes a dossier.

## Why this stage exists

T1 and T2 search Reddit, X and GitHub for real users discussing a product.
Their fatal failure mode is the namesake problem: search *Linear* and you get
linear algebra; search *Cursor* and you get database cursors.

So the dossier's real payload isn't the description — it's the **discriminator**:

- the aliases and distinctive jargon worth searching for
- co-occurring terms that confirm a hit (and terms that rule one out)
- the *other things sharing the name*, each with an evidence URL
- an **ambiguity score**: the share of the name space the product doesn't own

Name collisions are observed, never recalled. The backend searches the bare
product name with no category context, partitions the results by domain, and
the leftovers are the collision set — so a collision that can't be traced to a
page we actually fetched gets dropped before it reaches you.

## Install

```bash
go build -o fieldnote ./cmd/fieldnote
```

## Use

Start the backend first (see its README), then:

```bash
./fieldnote --url https://cursor.com --name Cursor --repo getcursor/cursor
```

Run it bare to be prompted for each input:

```bash
./fieldnote
```

| Flag | Meaning |
|---|---|
| `--url` | Product website. The only required input. |
| `--name` | Product name. Derived from the site when omitted. |
| `--repo` | `owner/repo` or a full GitHub URL. Optional. |
| `--details` | Free-text description. Optional, but the most precise disambiguation signal available. |
| `--backend` | Backend base URL. Defaults to `$FIELDNOTE_BACKEND` or `http://127.0.0.1:8000`. |
| `--json` | Print the dossier to stdout and skip the interactive UI. |
| `--out` | Output directory. Defaults to `out`. |

Output lands in `out/<slug>/`:

- `product.json` — the schema T1/T2 consume
- `product.md` — the same data for a human to sanity-check before spending scrape budget

Piped or redirected output automatically drops the animated UI, so
`./fieldnote --url acme.dev > run.log` behaves.

## Types are generated

`internal/client/types.gen.go` is generated from the backend's pydantic models —
don't hand-edit it. To refresh after a backend schema change:

```bash
cd ../field-note-backend && uv run python scripts/export_openapi.py > openapi.json
```

```bash
cp ../field-note-backend/openapi.json . && go tool oapi-codegen -config oapi-codegen.yaml openapi.json
```

## Test

```bash
go test ./...
```

Golden markdown lives in `internal/render/testdata`. Rewrite it with
`go test ./internal/render -update` after an intentional formatting change.
