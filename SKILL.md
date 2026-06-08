---
name: jazmem
description: Use jazmem CLI and markdown memory. Trigger when searching, reading, citing, storing, organizing, reindexing, or maintaining durable personal memory in jazmem.
---

# Jazmem

Jazmem is a markdown-first memory system. Markdown files are the source of truth. SQLite is only a rebuildable index for search, aliases, links, chunks, and scheduler state.

Use this skill whenever personal context may matter, whenever the user asks you to remember/store/file something, or after substantial work produces knowledge that should not live only in chat.

## Core Contract

- Check memory before answering questions about people, projects, preferences, prior decisions, open loops, and "what do we know" topics.
- Ground memory-based claims in citations. If memory is missing or thin, say so.
- Preserve the user's original wording when storing ideas, preferences, decisions, and concerns.
- Store uncertain material in `inbox/`; promote only durable, sourced facts to canonical pages.
- Use wikilinks for durable relationships and important references.
- After writing or editing markdown, run `jazmem index` and verify with search.
- Let `jazmem` perform all indexing. Agents edit markdown; agents do not edit SQLite, FTS rows, chunks, aliases, links, unresolved links, or scheduler/index state.
- Never store canonical facts only in SQLite.

## Defaults

No flags or env vars:

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

If `--root` is custom and `--db` is omitted, db defaults to:

```text
<root>/.jazmem/index.sqlite
```

Bootstrap the default memory root:

```bash
jazmem init
```

Bootstrap a custom memory root:

```bash
jazmem init /path/to/memory
jazmem init --root /path/to/memory
jazmem init --path /path/to/memory
```

`init` creates the markdown folder structure, initializes the rebuildable SQLite index, runs an initial reindex, and returns a JSON report. It is safe to run again.
It also initializes a git repo at the memory root when one is not already present and maintains a `.gitignore` for derived index files.

## Operating Loop

Use this sequence for most memory work:

1. Search: `jazmem "<names concrete nouns question>"`
2. Read full pages only when search results show they matter: `jazmem get --raw <slug>`
3. Answer from retrieved context, citing slugs and source lines when possible.
4. If the user gives new information, write it into the appropriate raw markdown file with exact wording and citation.
5. Decide whether it belongs in canonical pages.
6. If editing canonical pages, add citations and wikilinks.
7. Run `jazmem index`.
8. Verify retrieval: search for the person/project/topic again.

This mirrors the best gbrain practice: search retrieves context, full-page reads deepen only confirmed hits, and writes happen only when sourced and useful.

## Search

Search directly:

```bash
jazmem "Alice Riley Acme"
```

Explicit form:

```bash
jazmem search "Alice Riley Acme"
```

Limit results:

```bash
jazmem --limit 5 "Alice open loops"
```

Print assembled context text:

```bash
jazmem --text "what is open for Alice"
```

Search returns a retrieval envelope. It does not call an LLM yet.

```json
{
  "query": "Alice Riley Acme",
  "context": "# Retrieved Memory Context\n...",
  "citations": [
    {
      "slug": "people/alice",
      "title": "Alice Smith",
      "path": "/Users/wins/.jaz/memory/people/alice.md",
      "chunk_index": 0,
      "snippet": "Alice and [Riley] are friends...",
      "score": -0.00000157
    }
  ],
  "pages_gathered": 1,
  "chunks_gathered": 1,
  "warnings": [],
  "diagnostics": {
    "pages_from_bm25": 1,
    "chunks_from_bm25": 1,
    "mode": "bm25"
  },
  "results": [
    {
      "slug": "people/alice",
      "title": "Alice Smith",
      "chunk_index": 0,
      "snippet": "Alice and [Riley] are friends...",
      "score": -0.00000157
    }
  ]
}
```

Interpretation:

- `context` is assembled retrieval context for the agent or model to read.
- `citations` are the grounding chunks; cite their slugs when answering.
- `pages_gathered` counts unique pages in the BM25 result set.
- `chunks_gathered` counts matched chunks.
- `score` is SQLite BM25. Lower is better.
- `title` comes from frontmatter `title`, then first `# H1`, then slug tail.
- `slug` is the markdown path under root without `.md`.

## Search Strategy

Pick queries based on the question:

- Person context: `jazmem "Alice preferences decisions open loops"`
- Relationship lookup: `jazmem "Alice Riley friends"`
- Project context: `jazmem "jazmem sqlite bm25 vector"`
- Recent inbox/source note: `jazmem "Acme launch timeline"`
- Agent trace: `jazmem "agent solved problem failure fix"`
- User preference: `jazmem "user preference writing style"`

Rules:

- Use concrete names, aliases, project names, dates, and nouns.
- If results are thin, retry with alternate names or slugs.
- For "tell me about X", read the canonical page after search.
- For "did anyone mention X", search results may be enough.
- For relationship questions, search both entities together and read both pages when present.
- Do not answer from general memory when jazmem has relevant context.

## Read Memory

Get page JSON:

```bash
jazmem get people/alice
```

Get raw markdown:

```bash
jazmem get --raw people/alice
```

Get body without frontmatter:

```bash
jazmem get --body people/alice
```

Get canonical file path:

```bash
jazmem file people/alice
```

Use `file` before manual edits.

If the slug is not found, `jazmem file` and `jazmem get` return similar slugs:

```text
jazmem: not found: people/alice
suggestions:
- people/alice-bentick (Alice Bentick)
- people/alice-chen (Alice Chen)
```

When this happens, retry with the best matching slug before concluding memory is missing.

## Store Data

Agents store data by editing raw markdown files. This is the default and preferred write path.
The agent writes markdown; jazmem indexes it.

Use the CLI to locate and verify memory, then edit files in `<root>`:

```bash
jazmem doctor
jazmem "Alice concise launch updates"
jazmem file people/alice
jazmem get --raw people/alice
```

Write workflow:

1. Search for existing pages before writing.
2. Use `jazmem file <slug>` to get the canonical file path for existing pages.
3. Edit the markdown file directly with the available filesystem editing tool.
4. For new pages, create `<root>/<slug>.md` with frontmatter and an H1.
5. Preserve exact user wording for ideas, preferences, decisions, and concerns.
6. Add source citations to every durable fact.
7. Run `jazmem index`; this is the only indexing step agents should perform.
8. Verify with a search query that should find the new memory.

For raw or uncertain information, create an inbox/source markdown file directly:

```md
---
title: Alice launch update preference
type: inbox
created: 2026-06-08T12:00:00Z
source: chat
---

# Alice launch update preference

Alice said she prefers concise launch updates with explicit open questions.

[Source: User, chat, 2026-06-08]
```

Good raw locations:

- `inbox/YYYY-MM-DD-short-topic.md` for untriaged user context
- `daily/YYYY-MM-DD.md` for dated operating notes
- `sources/chat/YYYY-MM-DD-topic.md` for chat source material
- `sources/email/YYYY-MM-DD-topic.md` for email source material
- `sources/agent/YYYY-MM-DD-topic.md` for agent work traces

Edit canonical markdown when the fact is durable and sourced:

- preference
- decision
- relationship
- current role
- recurring open loop
- stable project context
- reusable concept
- important agent learning

Before creating a canonical page:

1. Search for the entity/topic by name and aliases.
2. If a page exists, update it instead of creating a duplicate.
3. If no page exists, apply the notability gate.
4. Pick the most specific directory and slug.
5. Create the page with frontmatter, H1, citations, and wikilinks.
6. Run `jazmem index`.
7. Verify the page is findable by name and alias.

After direct edits:

```bash
jazmem index
jazmem "keywords that should find the new memory"
jazmem checkpoint "stored alice launch preference"
```

Do not manually update the SQLite database after markdown edits. If search looks stale, run `jazmem index` again and then `jazmem doctor`.
Use `jazmem checkpoint "<message>"` to commit markdown progress after the index/search verification passes.

## Page Taxonomy

Use these directories unless the user establishes a new taxonomy:

- `people/` - people the user may refer to again
- `companies/` - companies, orgs, institutions
- `projects/` - ongoing projects and tools
- `concepts/` - reusable mental models and ideas
- `notes/` - durable notes without a stronger home
- `daily/` - daily rollups and dated operating notes
- `sources/email/` - imported email source material
- `sources/chat/` - imported chat source material
- `sources/agent/` - agent traces and problem-solving logs
- `inbox/` - raw markdown notes waiting for triage
- `dreams/runs/` - dream run records
- `dreams/review/` - ambiguous dream candidates

Slug rules:

- Lowercase, path-like, no `.md` in the slug.
- Examples: `people/alice-smith`, `projects/jazmem`, `concepts/brain-first-memory`.
- Slug maps to `<root>/<slug>.md`.

## Canonical Page Shape

A good canonical page is easy to search, cite, and update.

```md
---
title: Alice Smith
type: people
aliases: [Alice]
---

# Alice Smith

## Current

- Runs engineering at Acme. [Source: [[sources/chat/2026-06-08-alice-context]], 2026-06-08]

## Preferences

- Prefers concise launch updates with explicit open questions. [Source: User, chat, 2026-06-08]

## Relationships

- [[people/riley]] - friend. [Source: [[inbox/2026-06-08-lunch-note]], 2026-06-08]

## Open Loops

- Ask about Acme launch timeline. [Source: [[inbox/2026-06-08-acme-launch]], 2026-06-08]

## Sources

- [[sources/chat/2026-06-08-alice-context]]
- [[inbox/2026-06-08-lunch-note]]
```

Recommended sections by page type:

- People: `Current`, `Preferences`, `Relationships`, `Open Loops`, `Sources`
- Companies: `Current`, `People`, `Projects`, `Open Loops`, `Sources`
- Projects: `State`, `Decisions`, `Design`, `Open Loops`, `Sources`
- Concepts: `Summary`, `Use When`, `Examples`, `Related`, `Sources`
- Agent traces: `Problem`, `Approach`, `Fix`, `Lessons`, `Sources`

Keep pages scannable. Prefer bullets for factual memory. Avoid long unsourced prose.

## Source And Citation Rules

Every durable fact written to a canonical page should include a source.

Use these citation forms:

- User statement: `[Source: User, chat, 2026-06-08]`
- Source page: `[Source: [[inbox/2026-06-08-note]], 2026-06-08]`
- Email: `[Source: email from Alice re: Acme launch, 2026-06-08]`
- Chat: `[Source: chat with Alice, 2026-06-08]`
- Agent work: `[Source: [[sources/agent/2026-06-08-jazmem-implementation]], 2026-06-08]`
- Web or external: `[Source: Publication, URL, 2026-06-08]`

Source precedence when sources conflict:

1. User direct statements
2. Canonical memory pages with citations
3. Raw inbox/source markdown pages
4. External sources

When memory conflicts, do not silently choose one. Preserve both claims with dates and sources, then flag the contradiction.

## Memlinks And Relationships

Use explicit wikilinks for durable references:

```md
[[people/alice]]
[[people/alice|Alice]]
[[projects/jazmem]]
```

Use `## Relationships` for stable relationships only:

```md
- [[people/riley]] - friend. [Source: [[inbox/2026-06-08-lunch-note]], 2026-06-08]
```

Rules:

- Do not create reciprocal relationship bullets for ordinary mentions.
- Do create reciprocal relationship bullets for durable relationships such as friend, works with, founder, advisor, investor, collaborator.
- If unsure, write a raw markdown note to `inbox/` or `dreams/review/` instead of editing canonical pages.
- After creating a new entity page, run `jazmem link-hygiene` to generate relationship proposals in `dreams/review/`.
- Promote a proposal only by manually editing the canonical markdown pages, then run `jazmem index`.

## Acquiring Context While Working

When the user gives you context during a task, classify it immediately:

1. Raw observation - write exact wording into an `inbox/`, `daily/`, or `sources/*` markdown file.
2. Durable fact - update canonical page with citation.
3. Relationship - update `## Relationships` on both relevant pages when high confidence.
4. Preference - update the person/user/project page with date and source.
5. Decision - update the project page under `## Decisions`.
6. Open loop - update the relevant page under `## Open Loops`.
7. Reusable process - update `concepts/` or `sources/agent/` as an agent trace.

Do not wait until the end of a long session if the information is important. Write raw markdown early, promote carefully.

For significant work sessions, produce memory artifacts:

- A source page under `sources/agent/` describing the problem, approach, commands or files changed, result, and lessons.
- Canonical project updates under `projects/`.
- New concepts only when the pattern is reusable.
- New people/company pages only when they pass the notability gate.

Notability gate:

- People: likely to recur, connected to user work, preferences, decisions, or relationships.
- Companies: relevant to user work, investments, projects, or recurring references.
- Concepts: reusable mental model or repeated theme.
- Otherwise: keep it in `inbox/` or a source page.

## Response Types

`jazmem <query>` and `jazmem search <query>` return `RetrievalContext`:

- `query`: original query
- `context`: assembled markdown-style context
- `citations`: slug/path/snippet refs
- `pages_gathered`, `chunks_gathered`: retrieval counts
- `diagnostics`: retrieval source counts and mode
- `results`: raw ranked BM25 chunk hits

`jazmem get <slug>` returns `Page`:

- `slug`, `path`, `type`, `title`, `aliases`
- `frontmatter`, `body`, `raw`, `modified_at`

`jazmem file <slug>` returns plain text:

- canonical markdown file path

`jazmem checkpoint "<message>"` returns `CheckpointReport`:

- `repo_path`
- `committed`
- `commit`
- `message`
- `files_added`

`jazmem index`, `jazmem dream`, `jazmem link-hygiene`, and `jazmem doctor` return JSON reports.

## Maintenance

Rebuild SQLite from markdown:

```bash
jazmem index
```

This command is the indexing boundary. It parses markdown, extracts frontmatter, aliases, wikilinks, mentions, chunks, unresolved links, and refreshes FTS/BM25 state.

Inspect counts and paths:

```bash
jazmem doctor
```

Run deterministic dream scaffold:

```bash
jazmem dream
```

Generate relationship review proposals:

```bash
jazmem link-hygiene
```

Basic health check after writes:

```bash
jazmem doctor
jazmem index
jazmem "known new keywords"
```

## Server

Start server:

```bash
jazmem-server --addr 127.0.0.1:9477
```

Search:

```bash
curl 'http://127.0.0.1:9477/search?q=Alice%20Riley&limit=5'
```

Read raw markdown:

```bash
curl 'http://127.0.0.1:9477/file/people/alice?raw=1'
```

## Anti-Patterns

- Answering from general knowledge when jazmem has relevant memory.
- Writing canonical facts without sources.
- Paraphrasing the user's original idea when writing raw inbox/source markdown.
- Creating pages for one-off, non-notable entities.
- Burying relationships in prose instead of `## Relationships`.
- Creating reciprocal links for mere mentions.
- Forgetting `jazmem index` after manual markdown edits.
- Treating SQLite as source of truth.
- Making a page so broad that future agents cannot tell where facts belong.
