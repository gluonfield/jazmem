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
- compact search responses with page results, matched chunks, and stats
- title/alias candidate generation, typed relationship retrieval, and one-hop memlink/backlink expansion
- simple scheduler/dream/link-hygiene scaffolding

The gbrain features most likely to matter for retrieval quality are:

1. Entity candidate generation: exact title/alias hits are high signal for personal-memory queries.
2. BM25 chunk retrieval: still the cheapest broad recall path.
3. Memlink graph expansion: explicit links, backlinks, and mention links should expand around the strongest pages.
4. Reranking: useful only after an eval shows candidate order is the limiting problem.
5. Evaluation: a fixed personal benchmark is needed before adding embeddings or tuning boosts.
6. Incremental indexing: required once the corpus grows, but not the main quality driver.
7. Dream consolidation: useful only if it edits canonical pages conservatively with citations and review queues.

Current jazmem retrieval uses title/alias matching, BM25 chunks with per-page max-pool, typed relationship retrieval, page-level merging, and one-hop memlink/backlink expansion. Agentic answer synthesis is an OpenRouter layer over those retrieved results. It does not use embeddings or a reranker.

Do not copy gbrain wholesale. These are not v1 performance requirements for jazmem:

- 20+ phase nightly cycle machinery
- remote sync/federation layers
- job queues/minion infrastructure
- broad boost/RRF tuning before an eval says it helps
- embeddings before synonym recall is a demonstrated problem
- typed edge extraction before untyped memlink/backlink retrieval is working

The target retrieval flow is:

```text
query
-> candidate generation: title/alias exact, BM25 chunks
-> relational arm: typed edges for relationship-shaped queries
-> merge by slug: one page result with matched evidence
-> graph expansion: explicit links, backlinks, and mentions around strongest pages
-> optional future rerank top candidates
-> return compact raw results or OpenRouter-backed agentic answer
```

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
jazmem --agentic "what do we know about Leeroo"
jazmem "who works at Acme"
jazmem "what connects Alice and Widget Co"
jazmem eval
```

`--agentic` calls OpenRouter and requires `OPENROUTER_API_KEY`. `jazmem` loads `.env` from the current tree when present, including `jaz/backend/.env` in this workspace.

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

Run maintenance:

```bash
jazmem dream
jazmem link-hygiene
```

`dream` calls OpenRouter. It writes a dream run page, appends only validated high-confidence bullets to existing canonical pages, and sends ambiguous items to `dreams/review/`.

## Server

Run:

```bash
jazmem-server
```

Default address:

```text
127.0.0.1:9477
```

Endpoints:

```bash
curl 'http://127.0.0.1:9477/health'
curl 'http://127.0.0.1:9477/doctor'
curl 'http://127.0.0.1:9477/search?q=Ink%20enterprise&limit=5'
curl 'http://127.0.0.1:9477/search?q=Ink%20enterprise&agentic=1'
curl 'http://127.0.0.1:9477/file/projects/ink'
curl 'http://127.0.0.1:9477/file/projects/ink?raw=1'
curl -X POST 'http://127.0.0.1:9477/reindex'
curl -X POST 'http://127.0.0.1:9477/dream'
curl -X POST 'http://127.0.0.1:9477/link-hygiene'
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

`jazmem search` returns `SearchResponse`:

```json
{
  "results": [
    {
      "slug": "concepts/ink-enterprise-deployment-strategy",
      "title": "Ink Enterprise Deployment Strategy",
      "score": -4.79975572,
      "matches": [
        {
          "chunk": 0,
          "snippet": "Strongest current enterprise wedge...",
          "score": -4.79975572
        }
      ]
    }
  ],
  "stats": {
    "pages": 1,
    "chunks": 1
  }
}
```

This is raw ranked retrieval with matched chunk evidence. No chat model is called.

Agentic search:

```bash
jazmem --agentic "what do we know about Leeroo"
curl 'http://127.0.0.1:9477/search?q=Leeroo&agentic=1'
```

returns `AgenticResponse`:

```json
{
  "answer": "Leeroo is connected to Ink through the deployment collaboration...",
  "citations": [
    {
      "slug": "companies/leeroo",
      "title": "Leeroo",
      "chunk": 0
    }
  ],
  "stats": {
    "pages": 1,
    "chunks": 1
  },
  "model_used": "openai/gpt-5.4-mini",
  "rounds": 1,
  "synthesis_ok": true,
  "diagnostics": {
    "pages_gathered": 1,
    "chunks_gathered": 1,
    "graph_hits": 0
  }
}
```

This is OpenRouter-backed synthesis over retrieved markdown evidence. Raw retrieval remains deterministic and free; `--agentic` requires `OPENROUTER_API_KEY`.

Eval:

```bash
jazmem eval
jazmem eval --limit 10
jazmem eval --file ./my-eval.json
```

Eval uses raw retrieval and scores returned slugs against expected slugs. It reports hit rate, precision, recall, and MRR.

## Typed Relationships

Typed edges are derived from explicit wikilinks inside `## Relationships` sections. They are stored only in SQLite and rebuilt by `jazmem index`.

Good canonical shape:

```md
## Relationships

- [[companies/acme]] - works at. [Source: User, chat, 2026-06-08]
- [[companies/widget-co]] - invested in. [Source: User, chat, 2026-06-08]
- [[people/riley]] - friend. [Source: User, chat, 2026-06-08]
```

Supported v1 edge types:

- `works_at`
- `works_with`
- `founder_of`
- `invested_in`
- `advises`
- `friend`

Supported relational query forms include:

```bash
jazmem "who works at Acme"
jazmem "who invested in Widget Co"
jazmem "who founded Widget Co"
jazmem "what companies has Alice invested in"
jazmem "who are Alice's friends"
jazmem "what connects Alice and Widget Co"
```

No LLM is used for this path.

## What Is Not Implemented Yet

- Incremental indexing
- Embeddings
- Vector search
- Reranker
- Full ingestion connectors
- Durable workflow/checkpointing for dream beyond markdown run pages

These should be added behind the existing package/CLI surfaces without changing markdown as the source of truth.
