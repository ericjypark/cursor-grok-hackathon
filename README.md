# fieldnote

CLI frontend for the field-note pipeline: **T0 — product understanding** and
**T1 — finding where users complain**.

You give it a GitHub repo (plus, optionally, a product website and a free-text
description). It calls the [field-note backend](https://github.com/in-sol-ence/field-note-backend),
streams live progress, and writes a dossier plus the discussions it scraped.

## Why T0 exists

T1 searches Reddit and HackerNews for real users discussing a product. Its
fatal failure mode is the namesake problem: search *Linear* and you get
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

## What T1 does with it

The dossier is not the deliverable, it's the aim. T1 turns it into concrete
subreddits and search queries — spending the collisions, the negative signals
and the no-namesake jargon — then scrapes them and returns the discussions as
normalized posts.

The scrape runs through the social-signals service, which needs a live browser
session and takes 3-5 minutes for Reddit. When it is not reachable, the run
falls back to recorded signals and **says so**: the summary marks the posts
`recorded` instead of `live`. Those recordings are about Perplexity, so a
fallback run proves the plumbing — it does not tell you about your users.

Skip the scrape entirely with `--no-scrape`.

## Quick start

One command — clones the backend if needed, syncs it, starts it, builds the
CLI and runs it. Shuts the server down on exit unless one was already running.

```bash
./run.sh --url https://cursor.com --name Cursor --repo getcursor/cursor
```

Bare, to be prompted for each input:

```bash
./run.sh
```

Without API keys, use the canned dossier — real server, real CLI, real SSE, no
APIs called. T1 still runs for real against it, so this exercises the whole
scrape-then-fall-back path. Also worth using to rehearse a demo rather than
betting it on three external services staying up:

```bash
./run.sh --demo --repo getcursor/cursor
```

`--demo` always returns the same Cursor dossier whatever you pass it. Every
other flag is forwarded to the CLI untouched.

### Fully live (real T0 dossier + live T1 scrapes)

T1 can already be live under `--demo`. To also synthesize a **real** dossier
(Firecrawl + Exa + Grok), drop `--demo` and put keys in the backend `.env`:

```bash
cd ../field-note-backend && cp .env.example .env
# fill XAI_API_KEY, FIRECRAWL_API_KEY, EXA_API_KEY

# Reddit/HN (run.sh will try to start this if :8899 is down)
../field-note-backend/scripts/run_social_signals_lite.sh

FIELDNOTE_BACKEND_DIR=../field-note-backend FIELDNOTE_PORT=8001 \
  ./run.sh --repo getcursor/cursor --url https://cursor.com
```

Expect `/health` → `"mode":"live"`, and `out/<slug>/posts.json` →
`"source_note": "live scrape"`. Team checklist (ops pitfalls, fixture rules,
Chromium ordering): `field-note-backend/notes-gregory.md` §11.

Override the defaults with `FIELDNOTE_BACKEND_DIR` (defaults to a sibling
`field-note-backend`), `FIELDNOTE_PORT` (defaults to 8000) and
`FIELDNOTE_SCRAPER` (defaults to `http://127.0.0.1:8899`). `run.sh` probes the
scraper before it starts and tells you which mode T1 will be in, rather than
letting you find out halfway through a demo.

## Running the pieces by hand

```bash
go build -o fieldnote ./cmd/fieldnote
```

With the backend already up (see its README):

```bash
./fieldnote --url https://cursor.com --name Cursor --repo getcursor/cursor
```

| Flag | Meaning |
|---|---|
| `--url` | Product website. Optional. |
| `--name` | Product name. Derived from the repo or site when omitted. |
| `--repo` | `owner/repo` or a full GitHub URL. The only required input. |
| `--details` | Free-text description. Optional, but the most precise disambiguation signal available. |
| `--backend` | Backend base URL. Defaults to `$FIELDNOTE_BACKEND` or `http://127.0.0.1:8000`. |
| `--json` | Print the results to stdout and skip the interactive UI. |
| `--out` | Output directory. Defaults to `out`. |
| `--no-scrape` | Stop after T0. Builds the dossier, skips T1 entirely. |
| `--scrape-x` | Live X via x-scraper (default true). Use `-scrape-x=false` to disable. |
| `--scrape-social` | Reddit/HN via social-signals (default true). Use `-scrape-social=false` to disable. |

Output lands in `out/<slug>/`:

- `product.json` — the dossier, which is T1's input
- `product.md` — the same data for a human to sanity-check before spending scrape budget
- `posts.json` — the targets T1 chose and the discussions it scraped, which is
  T2's input. Absent on a `--no-scrape` run.

Piped or redirected output automatically drops the animated UI, so
`./fieldnote --repo acme/acme > run.log` behaves.

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
