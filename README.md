# jazmem

Markdown-first personal memory for `jaz`.

Markdown files are the source of truth. SQLite is a rebuildable index for FTS/BM25 search, aliases, links, chunks, scheduler state, and future optional embeddings. Agents store memory by editing raw markdown files, then running `jazmem index`.

## Current Performance Position

jazmem does not yet have performance parity with gbrain.

It has the right substrate for a smaller, cleaner version:

- canonical markdown pages
- git history for markdown memory
- SQLite FTS5/BM25 search through `modernc.org/sqlite`
- wikilink and mention extraction
- retrieval context with citations and diagnostics
- simple scheduler/dream/link-hygiene scaffolding

The gbrain features most likely to matter for retrieval quality are:

1. Memlink graph retrieval: explicit links and backlinks should expand the candidate set around the initial BM25 hits.
2. Reranking: a cross-encoder or strong LLM reranker is the biggest likely precision lift after candidate generation.
3. Evaluation: a fixed personal benchmark is needed before adding embeddings or tuning boosts.
4. Incremental indexing: required once the corpus grows, but not the main quality driver.
5. Dream consolidation: useful only if it edits canonical pages conservatively with citations and review queues.

Current jazmem retrieval is BM25-only over chunks, with strict-then-broad token matching. It does not use embeddings and does not use a reranker.

## Install

From the local repo:

```bash
cd /Users/wins/Projects/personal/jarvis/jazmem
go test ./...
go build -o ~/.local/bin/jazmem ./cmd/jazmem
go build -o ~/.local/bin/jazmem-server ./cmd/jazmem-server
```

`/Users/wins/.local/bin` must be in `PATH`.

Verify:

```bash
which jazmem
jazmem doctor
```

Install the jazmem skill for jaz agents:

```bash
mkdir -p ~/.jaz/skills/jazmem
cp /Users/wins/Projects/personal/jarvis/jazmem/SKILL.md ~/.jaz/skills/jazmem/SKILL.md
```

## Initialize Memory

Default memory root:

```text
~/.jaz/memory
```

Default SQLite index:

```text
~/.jaz/jazmem.sqlite
```

Initialize:

```bash
jazmem init
```

`init` creates the folder layout, initializes a git repo at the memory root if needed, maintains `.gitignore`, and rebuilds the SQLite index.

## Core Commands

Search:

```bash
jazmem "Ink enterprise Claude deployment"
jazmem search --limit 5 "Oxford Edge Irwin Zaid"
jazmem search --text "physics reasoning environment"
```

Read pages:

```bash
jazmem file projects/ink
jazmem get projects/ink
jazmem get --raw projects/ink
```

Rebuild index:

```bash
jazmem index
jazmem doctor
```

Commit markdown progress:

```bash
jazmem checkpoint "updated ink enterprise strategy"
```

Run maintenance scaffolding:

```bash
jazmem dream
jazmem link-hygiene
```

## Server

Run:

```bash
jazmem-server
```

Default address:

```text
127.0.0.1:8765
```

Endpoints:

```bash
curl 'http://127.0.0.1:8765/health'
curl 'http://127.0.0.1:8765/doctor'
curl 'http://127.0.0.1:8765/search?q=Ink%20enterprise&limit=5'
curl 'http://127.0.0.1:8765/file/projects/ink'
curl 'http://127.0.0.1:8765/file/projects/ink?raw=1'
curl -X POST 'http://127.0.0.1:8765/reindex'
curl -X POST 'http://127.0.0.1:8765/dream'
curl -X POST 'http://127.0.0.1:8765/link-hygiene'
```

There is no capture endpoint. Store data by editing markdown files.

## Store Data

Agents should:

1. Search for existing pages.
2. Resolve a file with `jazmem file <slug>`.
3. Edit the markdown file directly.
4. Create new markdown files only when no suitable page exists.
5. Add citations for durable facts.
6. Run `jazmem index`.
7. Verify with search.
8. Run `jazmem checkpoint "<message>"`.

Useful locations:

- `people/`
- `companies/`
- `projects/`
- `concepts/`
- `notes/`
- `daily/`
- `sources/email/`
- `sources/chat/`
- `sources/agent/`
- `inbox/`

## Search API Shape

`jazmem search` returns `RetrievalContext`:

```json
{
  "query": "Ink enterprise Claude deployment",
  "context": "# Retrieved Memory Context\n...",
  "citations": [],
  "pages_gathered": 0,
  "chunks_gathered": 0,
  "warnings": [],
  "diagnostics": {
    "pages_from_bm25": 0,
    "chunks_from_bm25": 0,
    "mode": "bm25"
  },
  "results": []
}
```

This is retrieval context, not answer synthesis. No chat model is called.

## What Is Not Implemented Yet

- Incremental indexing
- Embeddings
- Vector search
- Reranker
- Graph-expanded search
- Durable dream workflow
- Full ingestion connectors
- Answer synthesis with gap analysis

These should be added behind the existing package/CLI surfaces without changing markdown as the source of truth.
