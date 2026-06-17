# jazmem

Markdown-first personal memory for Jaz.

jazmem keeps durable memory in plain markdown and treats SQLite as a rebuildable
index/cache for search, aliases, links, chunks, and maintenance state. The
markdown tree is the source of truth.

## What It Does

- Stores memory as editable markdown pages.
- Indexes pages into SQLite FTS/BM25 search tables.
- Resolves titles, aliases, wikilinks, mentions, and typed relationship edges.
- Exposes memory through CLI, HTTP, and MCP surfaces.
- Supports maintenance workflows such as reindexing, dreaming, and link hygiene.

## Install

Requires Go 1.26 or newer.

```bash
go test ./...
go install ./cmd/jazmem ./cmd/jazmem-server
```

The installed binaries are:

- `jazmem`: CLI for local memory operations.
- `jazmem-server`: HTTP and MCP server.

## Initialize

By default, jazmem uses:

- Memory root: `~/.jaz/memory`
- SQLite index: `~/.jaz/jazmem.sqlite`

Create the memory layout and verify it:

```bash
jazmem init
jazmem doctor
```

`init` creates the expected markdown layout, including horizon files such as
`LONG_TERM.md` and `SHORT_TERM.md`.

## Memory Layout

Root horizon files summarize memory at different time scales:

- `LONG_TERM.md`: profile-level identity, major goals, deep standing preferences, and key relationships.
- `SHORT_TERM.md`: current focus, active projects, and open loops.
- `daily/YYYY-MM-DD.md`: raw daily log entries and recent events.

Normal pages live in typed directories such as `people/`, `companies/`,
`projects/`, `concepts/`, and `inbox/`.

## CLI

Search memory:

```bash
jazmem search "query"
jazmem search "query" --limit 5
jazmem search "query" --text
jazmem search "query" --deep
```

Read pages:

```bash
jazmem get projects/jaz
jazmem get --raw projects/jaz
jazmem file projects/jaz
```

Create or update pages:

Edit markdown files directly. Use `jazmem file <slug>` to resolve the path for
an existing page. Jaz's scheduler owns indexing for ordinary memory writes.

Maintain the index:

```bash
jazmem index
jazmem doctor
jazmem link-hygiene
```

Run agentic retrieval:

```bash
jazmem ask "What should I know about this project?"
jazmem --agentic "What changed recently?"
jazmem --agentic --deep "What changed recently?"
```

See `references/commands.md` for the fuller command reference.

## Server

Run the local server:

```bash
jazmem-server --addr 127.0.0.1:8765
```

The server exposes read and search endpoints for local integrations. It uses the
same memory root and SQLite index as the CLI unless flags or environment
variables override them.

## MCP

jazmem can also run as an MCP server for agents that need structured access to
memory.

The MCP endpoint is served by `jazmem-server` at `/mcp`, for example
`http://127.0.0.1:9477/mcp`.

The MCP tools are intentionally compact so agent responses stay token-efficient:
search returns grounded snippets and citations, while page reads return the
markdown content needed for the task.

## Environment

Most settings can be provided by flags. Environment variables are useful for
local installs and service processes:

- `JAZMEM_ROOT`: markdown memory root.
- `JAZMEM_DB`: SQLite index path.
- `JAZMEM_SERVER`: server URL used by the CLI.
- `PROVIDER_ENDPOINT` or `JAZMEM_PROVIDER_ENDPOINT`: OpenAI-compatible provider endpoint.
- `MODEL` or `JAZMEM_MODEL`: model for provider-backed commands.
- `REASONING_EFFORT` or `JAZMEM_REASONING_EFFORT`: optional reasoning effort.
- `OPENROUTER_API_KEY` or `OPENAI_API_KEY`: provider credentials.

Keep `.env` files local. They are ignored by git.

## Development

Common checks:

```bash
gofmt -w .
go test ./...
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

SQLite schema changes live in Goose migrations under
`internal/store/sqlite/migrations`. Static SQL should live under
`internal/store/sqlite/queries/<concern>` and be regenerated with SQLC.

Generated files under `internal/store/sqlite/generated` are machine-owned; edit
the migration or query source instead.
