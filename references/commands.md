# Jazmem Commands And Responses

Use this reference when you need exact CLI commands, response shapes, server endpoints, eval behavior, or maintenance details.

## Core Commands

Search:

```bash
jazmem "Ink enterprise Claude deployment"
jazmem search --limit 5 "Oxford Edge Irwin Zaid"
jazmem search --text "physics reasoning environment"
jazmem --agentic "what do we know about Leeroo"
jazmem "who works at Acme"
jazmem "what connects Alice and Widget Co"
```

Read pages:

```bash
jazmem file projects/ink
jazmem get projects/ink
jazmem get --raw projects/ink
jazmem get --body projects/ink
```

Rebuild and inspect:

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
jazmem eval
jazmem eval --limit 10
jazmem eval --file ./my-eval.json
```

## Raw Search Response

`jazmem <query>` and `jazmem search <query>` return `SearchResponse`:

```json
{
  "results": [
    {
      "slug": "people/alice",
      "title": "Alice Smith",
      "score": -0.00000157,
      "matches": [
        {
          "chunk": 0,
          "snippet": "Alice and Riley are friends...",
          "score": -0.00000157
        }
      ]
    }
  ],
  "stats": {
    "pages": 1,
    "chunks": 1,
    "graph_hits": 0
  }
}
```

Interpretation:

- `results`: ranked page-level hits.
- `matches`: matched chunks under each page.
- `stats.pages`: returned pages.
- `stats.chunks`: returned matched chunks.
- `stats.graph_hits`: extra pages added by link/backlink expansion.
- `score`: best score for the page. Lower is better for SQLite BM25.
- `title`: frontmatter `title`, then first `# H1`, then slug tail.
- `slug`: markdown path under root without `.md`.

## Agentic Response

`jazmem --agentic <query>` returns `AgenticResponse`:

```json
{
  "answer": "Alice and Riley are friends and have worked on jazmem search.",
  "citations": [
    {
      "slug": "people/alice",
      "title": "Alice Smith",
      "chunk": 0
    }
  ],
  "gaps": [],
  "stats": {
    "pages": 12,
    "chunks": 13,
    "graph_hits": 1
  },
  "model_used": "openai/gpt-5.4-mini",
  "rounds": 1,
  "synthesis_ok": true,
  "diagnostics": {
    "pages_gathered": 12,
    "chunks_gathered": 13,
    "graph_hits": 1
  }
}
```

Rules:

- Agentic mode requires `JAZMEM_API_KEY`.
- Agentic mode uses its own internal retrieval budget. `--limit` does not control it.
- Raw search remains deterministic and does not call an LLM.
- Use agentic mode when answering the user directly; use raw search when selecting pages to read or edit.

Provider env:

- `JAZMEM_PROVIDER_ENDPOINT`: OpenAI-compatible base endpoint; defaults to `https://openrouter.ai/api/v1`.
- `JAZMEM_API_KEY`: provider API key.
- `JAZMEM_MODEL`: chat model; defaults to `openai/gpt-5.4-mini`.
- `JAZMEM_REASONING_EFFORT`: optional; sent as `reasoning_effort` when set.

## Other Responses

`jazmem get <slug>` returns `Page`:

- `slug`, `path`, `type`, `title`, `aliases`
- `frontmatter`, `body`, `raw`, `modified_at`

`jazmem file <slug>` returns the canonical markdown file path as plain text.

`jazmem checkpoint "<message>"` returns `CheckpointReport`:

- `repo_path`
- `committed`
- `commit`
- `message`
- `files_added`

`jazmem index`, `jazmem dream`, `jazmem link-hygiene`, `jazmem eval`, and `jazmem doctor` return JSON reports.

`jazmem index` includes `typed_links`; `jazmem doctor` includes `typed_link_count`.

## Eval

Eval does not call an LLM. It checks expected slugs against raw retrieval results.

```bash
jazmem eval
jazmem eval --limit 3
jazmem eval --file ./my-eval.json
```

Reports:

- `hit_rate`
- `precision`
- `recall`
- `mrr`
- per-case returned slugs and expected slugs

Use eval before adding retrieval machinery such as embeddings, reranking, or boost tuning.

## Server

Start server:

```bash
jazmem-server --addr 127.0.0.1:9477
```

Endpoints:

```bash
curl 'http://127.0.0.1:9477/health'
curl 'http://127.0.0.1:9477/doctor'
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&limit=5'
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&agentic=1'
curl 'http://127.0.0.1:9477/file/people/alice'
curl 'http://127.0.0.1:9477/file/people/alice?raw=1'
curl -X POST 'http://127.0.0.1:9477/reindex'
curl -X POST 'http://127.0.0.1:9477/dream'
curl -X POST 'http://127.0.0.1:9477/link-hygiene'
```

There is no capture endpoint. Store data by editing markdown files.

## MCP Server

Start the stdio MCP server:

```bash
jazmem-mcp
jazmem-mcp --root ~/.jaz/memory --db ~/.jaz/jazmem.sqlite
```

MCP client config:

```json
{
  "mcpServers": {
    "jazmem": {
      "command": "/Users/wins/.local/bin/jazmem-mcp",
      "args": []
    }
  }
}
```

Tools:

- `jazmem_search`: input `{ "query": "..." }`; output `AgenticResponse`; requires `JAZMEM_API_KEY`.
- `jazmem_get`: input `{ "slug": "people/alice" }`; primary text content is raw markdown. Structured output is `{ "found": true, "slug": "...", "path": "...", "title": "...", "raw": "..." }` or `{ "found": false, "error": "not found: people/alice", "suggestions": [...] }`.

MCP is read-only. There is no MCP write/capture/index/dream/checkpoint tool. To store data, edit markdown directly. Indexing, dreaming, link hygiene, and checkpointing are CLI/server/scheduler operations.
