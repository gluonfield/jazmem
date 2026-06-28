---
name: jazmem
description: Use jazmem CLI and markdown memory. Trigger when searching, reading, citing, storing, organizing, or maintaining durable personal memory in jazmem, including the LONG_TERM/SHORT_TERM/daily memory horizons.
metadata:
  short-description: Search and maintain jazmem memory
---

# Jazmem

Markdown-first personal memory: markdown files are the source of truth, SQLite is a rebuildable index. Default root `~/.jaz/memory`, db `~/.jaz/jazmem.sqlite` (override with `JAZMEM_ROOT`/`JAZMEM_DB` or `--root`/`--db`). You read and search through tools; you **write by editing markdown directly** — there is no capture/write tool on either surface.

Two surfaces, same memory:

- **MCP** — how jaz agents use it: tools `memory_search` and `memory_get_page`, served at `:5299/mcp/jaztools`. Read-only.
- **CLI** (`jazmem …`) — terminal/human use: auto-connects to the running jaz server (`:5299/jazmem`) so one process owns index writes. `--local` forces direct DB access; editing markdown always works directly.

## Memory Horizons

`LONG_TERM.md` and `SHORT_TERM.md` sit at the memory root and are injected into every jaz turn alongside today's `daily/` page. They are not indexed pages.

| Surface | Holds | You (agent) | Dream (periodic) |
|---|---|---|---|
| `LONG_TERM.md` | profile-level identity, biography, major goals, deep standing preferences, key relationships | **read-only** | sole writer; only facts meeting the long-term bar |
| `SHORT_TERM.md` | current focus, active projects, open loops | **update in place, live** when the present changes | prunes stale entries |
| `daily/YYYY-MM-DD.md` | raw log of today under `## Notes` / `## Open Loops` | **append as you go**, mid-session | reads, never writes |

- SHORT_TERM says what is true *now* and gets overwritten; daily/ says what *happened* and never does. Amend a daily bullet only if it was wrong or superseded the same day.
- Never edit `LONG_TERM.md`. To surface something for it, record it in daily/ with a citation; dream promotes only what meets the bar. It is not a changelog, coding-style file, decision log, or contact list — routine prefs, project corrections, and weak one-off contacts go to daily/, SHORT_TERM, or canonical pages.

## What To Capture

Capture immediately when you learn something durable — don't defer to session end. Memory is compressed insight, not a transcript. Classify as you go:

- **Raw or uncertain** → exact wording into `inbox/`, `daily/`, or `sources/*`.
- **Durable** → the canonical page, with a citation. This includes facts about people/companies/projects/network; who said what, who is working on what, who is blocked / excited / unhappy / aligned; decisions, preferences, goals, open loops, commitments, and relationship changes.

For significant work sessions, leave artifacts: a `sources/agent/` page (problem, approach, files/commands, result, lessons) plus canonical `projects/` updates. New `people/`/`companies/`/`concepts/` pages only when they pass the **notability gate**:

- People: likely to recur, tied to user work/prefs/decisions/relationships.
- Companies: relevant to user work, investments, projects, or recurring references.
- Concepts: a reusable model or repeated theme.
- Otherwise keep it in `inbox/` or a source page.

## Searching

Check memory before answering about people, companies, projects, preferences, decisions, relationships, open loops, or "what do we know".

| Goal | MCP (jaz agents) | CLI |
|---|---|---|
| Answer a question | `memory_search {query, deep?}` → cited answer + gaps | `jazmem ask "…"` (`--deep`) |
| Find pages to read/edit | use `memory_get_page` after | `jazmem "…"` / `jazmem search "…"` (raw ranked) |
| Relational | `memory_search {query:"who works at Acme"}` | `jazmem "who works at Acme"`, `jazmem "what connects Alice and Riley"` |

- `ask`/`memory_search` answer the user (LLM synthesis, needs the provider key); raw `jazmem search` finds pages deterministically (no LLM) — use it when selecting pages to read or edit.
- `--deep` / `deep:true` is the only compute knob: wider candidate pool + two-hop link expansion, and a gap-driven second round for answers. An escalation, not a default. `--limit` doesn't affect answer mode.
- Raw hits carry `modified_at` (staleness) and, when it matters, `via`: `relationship` (typed-edge match) or `link` (expansion neighbor, not a direct hit). `dreams/` pages are excluded from search; read them by slug.

**When search misses:** reformulate with concrete nouns; try name variants and relational forms; `jazmem search --deep --limit 20 "…"`; read the closest hit raw and follow its wikilinks/`links`/`backlinks`; only then call it missing — and if a legitimate variant failed, add it as an alias.

## Reading Pages

| Goal | MCP | CLI |
|---|---|---|
| Read a page | `memory_get_page {path:"people/alice"}` → raw markdown + metadata, links/backlinks, near-miss suggestions | `jazmem get people/alice` (JSON), `jazmem get --raw` (markdown), `jazmem get --body` |
| Get the file path to edit | — | `jazmem file people/alice` |

Both return the page's graph neighborhood — `links`/`backlinks` as `{slug, type, source}`, where type is `reference`, `mention`, or a typed relationship like `works_at`. Hop along these for more context.

## Writing

You store data by editing raw markdown; jazmem indexes it. There is **no write tool** (CLI or MCP).

1. Search first for an existing page — update it rather than duplicate.
2. `jazmem file <slug>` for an existing page's path, or resolve `<root>/<slug>.md` for a new one.
3. Edit the markdown with your filesystem tool: frontmatter, H1, aliases, and a `[Source: …, YYYY-MM-DD]` on every durable fact. Preserve exact user wording for ideas, preferences, decisions, concerns.
4. New canonical pages must pass the notability gate; when unsure, write to `inbox/` instead.

The index updates on Jaz's schedule — verify file contents directly if you need certainty before it reindexes. Never edit SQLite. Commit with plain git only if the root is a repo and the user asks.

Raw/uncertain example (`inbox/YYYY-MM-DD-topic.md`):

```md
---
title: Alice launch update preference
type: inbox
source: chat
---

# Alice launch update preference

Alice prefers concise launch updates with explicit open questions. [Source: User, chat, 2026-06-08]
```

## Lanes & Slugs

`people/ companies/ projects/ concepts/ notes/` (canonical) · `tasks/` (tracked work) · `daily/ inbox/ sources/{email,chat,agent}/` (raw) · `dreams/{runs,review}/` (dream's bookkeeping).

Slugs are lowercase, path-like, no `.md` (`people/alice-smith`, `projects/jazmem`). Slug maps to `<root>/<slug>.md`.

## Canonical Page Shape

```md
---
title: Alice Smith
type: people
aliases: [Alice, A. Smith, alicedev]
---

# Alice Smith

## Current
- Runs engineering at Acme. [Source: [[sources/chat/2026-06-08-alice]], 2026-06-08]

## Preferences
- Prefers concise launch updates with explicit open questions. [Source: User, chat, 2026-06-08]

## Relationships
- [[people/riley]] - friend. [Source: [[inbox/2026-06-08-lunch]], 2026-06-08]

## Open Loops
- Ask about Acme launch timeline. [Source: [[inbox/2026-06-08-acme]], 2026-06-08]

## Sources
- [[sources/chat/2026-06-08-alice]]
```

Recommended sections:

- People: `Current`, `Preferences`, `Relationships`, `History`, `Open Loops`, `Sources`
- Companies: `Current`, `People`, `Projects`, `History`, `Open Loops`, `Sources`
- Projects: `State`, `Decisions`, `Design`, `Open Loops`, `Sources`
- Concepts: `Summary`, `Use When`, `Examples`, `Related`, `Sources`
- Agent traces: `Problem`, `Approach`, `Fix`, `Lessons`, `Sources`

Keep pages scannable — bullets, not prose. Past ~150 lines, move detail to a `sources/` page and keep a summarized bullet with a wikilink.

**Aliases** are the strongest retrieval signal, not optional metadata. Record every variant on creation (nicknames, initials, legal names, handles, product names); keep `title` canonical. When a reasonable search variant fails, add it as an alias. Never add generic words (`engineer`, `the company`).

## Citations

Every durable fact on a canonical page carries a source, with absolute dates only (`2026-06-09`, never "yesterday"):

- `[Source: User, chat, 2026-06-08]`
- `[Source: [[inbox/2026-06-08-note]], 2026-06-08]`
- `[Source: email from Alice re: Acme launch, 2026-06-08]`
- `[Source: Publication, URL, 2026-06-08]`

Precedence when sources conflict: user statements > cited canonical pages > raw inbox/source pages > external. When memory conflicts, don't silently pick — keep both claims with dates/sources and flag the contradiction.

## Updating Facts Over Time

Canonical pages describe the present. When a fact changes: update the `## Current` bullet in place (new fact, source, date) and move the displaced one to `## History` with a date range. Never leave a known-stale fact in `## Current`.

```md
## History
- Ran engineering at Acme (2024 to 2026-05). [Source: User, chat, 2026-06-09]
```

When a relationship ends, move its bullet from `## Relationships` to `## History` — that drops the typed edge while keeping the record. Update both pages of a reciprocal relationship.

## Relationships

Use explicit wikilinks for durable references: `[[people/alice]]`, `[[people/alice|Alice]]`, `[[projects/jazmem]]`.

Typed edges index **only** from `## Relationships` wikilink bullets with a supported label:

```md
## Relationships
- [[companies/acme]] - works at. [Source: User, chat, 2026-06-08]
- [[companies/widget-co]] - invested in. [Source: User, chat, 2026-06-08]
- [[people/riley]] - friend. [Source: User, chat, 2026-06-08]
```

Labels: `works at`→works_at · `works with`/`collaborator`→works_with · `founder`/`founded`→founder_of · `invested in`/`investor`→invested_in · `advisor`/`advises`→advises · `friend`→friend.

Create reciprocal bullets for durable relationships (friend, works with, founder, advisor, investor, collaborator) on both pages; don't for ordinary mentions. If unsure, write a raw note to `inbox/`.

## Tasks

The `tasks/` lane tracks forward-looking work: one flat page per task, `status` (`not-started` | `in-progress` | `done`) as a frontmatter field — **never the folder**, so the slug and its links survive the whole lifecycle. No `type` field; the folder is the type. jazmem seeds `tasks/SCHEMA.md` in every root as the authoritative shape.

```md
---
title: WhatsApp QR password continuation
status: in-progress      # not-started | in-progress | done
project: projects/jaz    # optional: the project this advances (bare slug)
opened: 2026-06-27
closed:                  # set the date when status becomes done
---

# WhatsApp QR password continuation

What it is and the next concrete action. Blockers and who you're waiting on go in the body, not frontmatter.

## Log
- 2026-06-27 opened
```

- **Create / advance / finish** — edit the markdown: write `tasks/<slug>.md`; flip `status` in place (the slug never moves); on done, set `status: done` and `closed`. Nothing is deleted — dream rolls a one-line residue onto the linked project page's `## Completed Tasks` and keeps the file.
- **Read a known task** — `memory_get_page tasks/<slug>` (CLI: `jazmem get tasks/<slug>`). Exact path, no search.
- **List the working set** — `jazmem tasks` (open by default; `--status done|all|<status>`). Don't enumerate tasks through `memory_search` — it ranks, it doesn't list.
- `project` is a page reference that rides the graph, so a project's backlinks show its open work.

## Maintenance & Anti-Patterns

Jaz's scheduler runs indexing, six-hour dream consolidation, and link hygiene automatically. Don't run maintenance commands during ordinary memory work unless explicitly asked; never treat SQLite as truth.

Avoid:

- Answering from general knowledge when memory has the answer; calling it "missing" after one search.
- Deferring capture to session end; editing `LONG_TERM.md` directly.
- Unsourced facts; relative dates; imperative phrasing ("always be concise"); IDs/SHAs that rot in a week.
- Alias-less new pages; stale `## Current` bullets; relationships buried in prose.
- `--deep` on every query.
