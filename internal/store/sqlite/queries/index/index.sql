-- name: ClearChunksFTS :exec
DELETE FROM chunks_fts;

-- name: ClearChunks :exec
DELETE FROM chunks;

-- name: ClearUnresolvedLinks :exec
DELETE FROM unresolved_links;

-- name: ClearLinks :exec
DELETE FROM links;

-- name: ClearAliases :exec
DELETE FROM aliases;

-- name: ClearPages :exec
DELETE FROM pages;

-- name: InsertPage :exec
INSERT INTO pages(
	slug,
	path,
	type,
	title,
	aliases_json,
	body_hash,
	frontmatter_json,
	modified_at_ms,
	indexed_at_ms,
	extractor_hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAlias :exec
INSERT OR IGNORE INTO aliases(slug, alias, normalized_alias)
VALUES (?, ?, ?);

-- name: InsertLink :exec
INSERT OR IGNORE INTO links(from_slug, to_slug, link_type, link_source, display, context)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertUnresolvedLink :exec
INSERT OR IGNORE INTO unresolved_links(from_slug, target, display, reason, context)
VALUES (?, ?, ?, ?, ?);

-- name: InsertChunk :exec
INSERT INTO chunks(slug, chunk_index, body, body_hash, embedding, modified_at_ms)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertChunkFTS :exec
INSERT INTO chunks_fts(slug, chunk_index, title, body)
VALUES (?, ?, ?, ?);

-- name: RecordIndexState :exec
INSERT OR REPLACE INTO index_state(key, value, updated_at_ms)
VALUES (?, ?, ?);
