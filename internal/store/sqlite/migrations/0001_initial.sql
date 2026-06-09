-- +goose Up
CREATE TABLE IF NOT EXISTS pages (
	slug TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	aliases_json TEXT NOT NULL,
	body_hash TEXT NOT NULL,
	frontmatter_json TEXT NOT NULL,
	modified_at_ms INTEGER NOT NULL,
	indexed_at_ms INTEGER NOT NULL,
	extractor_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS aliases (
	slug TEXT NOT NULL,
	alias TEXT NOT NULL,
	normalized_alias TEXT NOT NULL,
	PRIMARY KEY (slug, normalized_alias)
);

CREATE INDEX IF NOT EXISTS aliases_normalized_idx ON aliases(normalized_alias);

CREATE TABLE IF NOT EXISTS links (
	from_slug TEXT NOT NULL,
	to_slug TEXT NOT NULL,
	link_type TEXT NOT NULL,
	link_source TEXT NOT NULL,
	display TEXT NOT NULL,
	context TEXT NOT NULL,
	PRIMARY KEY (from_slug, to_slug, link_type, link_source, display, context)
);

CREATE INDEX IF NOT EXISTS links_to_idx ON links(to_slug);
CREATE INDEX IF NOT EXISTS links_type_idx ON links(link_type, from_slug, to_slug);

CREATE TABLE IF NOT EXISTS unresolved_links (
	from_slug TEXT NOT NULL,
	target TEXT NOT NULL,
	display TEXT NOT NULL,
	reason TEXT NOT NULL,
	context TEXT NOT NULL,
	PRIMARY KEY (from_slug, target, display, reason, context)
);

CREATE TABLE IF NOT EXISTS chunks (
	slug TEXT NOT NULL,
	chunk_index INTEGER NOT NULL,
	body TEXT NOT NULL,
	body_hash TEXT NOT NULL,
	embedding BLOB,
	modified_at_ms INTEGER NOT NULL,
	PRIMARY KEY (slug, chunk_index)
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
	slug UNINDEXED,
	chunk_index UNINDEXED,
	title,
	body
);

CREATE TABLE IF NOT EXISTS scheduler_state (
	task TEXT PRIMARY KEY,
	last_run_at_ms INTEGER NOT NULL,
	last_status TEXT NOT NULL,
	last_error TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS index_state (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at_ms INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS index_state;
DROP TABLE IF EXISTS scheduler_state;
DROP TABLE IF EXISTS chunks_fts;
DROP TABLE IF EXISTS chunks;
DROP TABLE IF EXISTS unresolved_links;
DROP TABLE IF EXISTS links;
DROP TABLE IF EXISTS aliases;
DROP TABLE IF EXISTS pages;
