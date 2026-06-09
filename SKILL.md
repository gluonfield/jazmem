---
name: jazmem
description: Use jazmem CLI and markdown memory. Trigger when searching, reading, citing, storing, organizing, reindexing, evaluating, dreaming, or maintaining durable personal memory in jazmem.
metadata:
  short-description: Search and maintain jazmem memory
---

# Jazmem

Jazmem is a markdown-first personal memory system. Markdown files are the source of truth; SQLite is only a rebuildable index for search, aliases, links, chunks, and scheduler state.

Use this skill whenever personal context may matter, when the user asks to remember/store/file something, or after substantial work produces durable knowledge that should not live only in chat.

## Core Rules

- Check jazmem before answering questions about people, projects, preferences, prior decisions, relationships, open loops, and "what do we know" topics.
- Ground memory-based claims in citations. If memory is missing or thin, say so.
- Agents store data by editing raw markdown files, then running `jazmem index`.
- Never store canonical facts only in SQLite and never edit SQLite/FTS/chunk/link tables directly.
- Preserve the user's original wording in raw notes and cite every durable fact promoted to canonical pages.
- Store uncertain material in `inbox/` or `dreams/review/`; promote only durable, sourced facts to canonical pages.
- Use wikilinks for durable references and relationships.
- After writing/editing markdown: run `jazmem index`, then verify with search.

## Defaults

No flags:

```bash
jazmem doctor
```

uses:

- root: `~/.jaz/memory`
- db: `~/.jaz/jazmem.sqlite`

Overrides:

- `JAZMEM_ROOT`
- `JAZMEM_DB`
- `--root`
- `--path` for `init`
- `--db`

Bootstrap memory:

```bash
jazmem init
jazmem init /path/to/memory
```

## Operating Loop

Use this sequence for most memory work:

1. Search first: `jazmem "<names concrete nouns question>"`
2. Read full pages only when search results show they matter: `jazmem get --raw <slug>`
3. Answer from retrieved context, citing slugs and source lines when possible.
4. If the user gives new information, write it into the appropriate markdown file with exact wording and citation.
5. Promote only durable, sourced facts to canonical pages.
6. Run `jazmem index`.
7. Verify retrieval with a search that should find the new memory.
8. Commit with plain git only if the memory root is a git repo and the user explicitly asks.

For detailed storage/page-shape rules, read [references/writing-memory.md](references/writing-memory.md).

## Search

Raw search:

```bash
jazmem "Alice Riley Acme"
jazmem search --limit 5 "Alice open loops"
jazmem --text "what is open for Alice"
```

Agentic synthesis:

```bash
jazmem --agentic "what do we know about Alice"
jazmem --agentic --text "what is open for Alice"
```

Raw search is deterministic and free. It uses title/alias candidates, BM25 chunks with per-page max-pool, typed relationship retrieval, and one-hop memlink/backlink expansion.

`--agentic` calls the configured OpenAI-compatible provider and requires `JAZMEM_API_KEY`. It uses its own internal context budget; do not use `--limit` to tune agentic retrieval. Use raw search when deciding which pages to read or edit.

Provider env:

- `JAZMEM_PROVIDER_ENDPOINT`
- `JAZMEM_API_KEY`
- `JAZMEM_MODEL`
- `JAZMEM_REASONING_EFFORT`

Search strategy:

- Person context: `jazmem "Alice preferences decisions open loops"`
- Relationship lookup: `jazmem "Alice Riley friends"`
- Typed relationship lookup: `jazmem "who works at Acme"`
- Connection lookup: `jazmem "what connects Alice and Riley"`
- Project context: `jazmem "jazmem sqlite bm25 vector"`
- Source/agent trace: `jazmem "agent solved problem failure fix"`
- User preference: `jazmem "user preference writing style"`

For response schemas, server endpoints, eval, and maintenance commands, read [references/commands.md](references/commands.md).

When MCP tools are available, prefer them over shell commands for read-only retrieval:

- `jazmem_search`
- `jazmem_get`

MCP is served by `jazmem-server` at `http://127.0.0.1:9477/mcp`; there is no separate jazmem MCP binary. `jazmem_search` is agentic by default and returns a cited answer/gaps. Use `jazmem_get` to read raw markdown pages by slug.

MCP is read-only. Indexing, dreaming, and link hygiene are CLI/server/scheduler operations, not MCP tools.

## Read Memory

```bash
jazmem get people/alice
jazmem get --raw people/alice
jazmem get --body people/alice
jazmem file people/alice
```

Use `jazmem file <slug>` before manual edits.

If a slug is not found, `jazmem file` and `jazmem get` return similar slugs:

```text
jazmem: not found: people/alice
suggestions:
- people/alice-bentick (Alice Bentick)
```

Retry with the best matching slug before concluding memory is missing.

## Write Memory

Default write path:

1. Search for existing pages.
2. Resolve the file path with `jazmem file <slug>`.
3. Edit markdown directly.
4. For new pages, create `<root>/<slug>.md` with frontmatter and an H1.
5. Add citations and wikilinks.
6. Run `jazmem index`.
7. Verify search.
8. Commit with plain git only when explicitly needed.

Canonical page directories:

- `people/`, `companies/`, `projects/`, `concepts/`, `notes/`
- `daily/`, `inbox/`, `sources/email/`, `sources/chat/`, `sources/agent/`
- `dreams/runs/`, `dreams/review/`

Use `## Relationships` for stable relationship bullets. Jazmem indexes typed relationship edges only from explicit wikilinks inside that section.

```md
## Relationships

- [[companies/acme]] - works at. [Source: User, chat, 2026-06-08]
- [[people/riley]] - friend. [Source: [[inbox/2026-06-08-lunch-note]], 2026-06-08]
```

Do not create reciprocal relationship bullets for mere mentions. Do create them for high-confidence durable relationships such as friend, works with, founder, advisor, investor, or collaborator.

## Maintenance

```bash
jazmem index
jazmem doctor
jazmem eval
jazmem dream
jazmem link-hygiene
```

- `jazmem index`: rebuilds SQLite from markdown.
- `jazmem doctor`: checks root/db/index counts.
- `jazmem eval`: fixed retrieval eval, no LLM.
- `jazmem dream`: provider-backed consolidation; writes run/review pages and only promotes validated cited bullets.
- `jazmem link-hygiene`: generates relationship proposals in `dreams/review/`.

## Anti-Patterns

- Answering from general knowledge when jazmem has relevant memory.
- Writing canonical facts without sources.
- Paraphrasing raw user ideas instead of preserving wording.
- Creating pages for one-off, non-notable entities.
- Burying durable relationships in prose instead of `## Relationships`.
- Forgetting `jazmem index` after manual markdown edits.
- Treating SQLite as source of truth.
