-- name: ListIndexedPages :many
SELECT slug, path, body_hash, frontmatter_json, modified_at_ms, extractor_hash FROM pages;

-- name: IndexCatalog :one
SELECT value FROM index_state WHERE key = 'catalog';

-- name: DeletePageIndex :exec
DELETE FROM pages WHERE slug IN (SELECT value FROM json_each(?));

-- name: DeletePageAliases :exec
DELETE FROM aliases WHERE slug IN (SELECT value FROM json_each(?));

-- name: DeletePageLinks :exec
DELETE FROM links WHERE from_slug IN (SELECT value FROM json_each(?));

-- name: DeletePageUnresolved :exec
DELETE FROM unresolved_links WHERE from_slug IN (SELECT value FROM json_each(?));

-- name: DeletePageChunks :exec
DELETE FROM chunks WHERE slug IN (SELECT value FROM json_each(?));

-- name: DeletePageChunksFTS :exec
DELETE FROM chunks_fts WHERE slug IN (SELECT value FROM json_each(?));

-- name: IndexCounts :one
SELECT
    (SELECT COUNT(*) FROM pages) AS pages,
    (SELECT COUNT(*) FROM chunks) AS chunks,
    (SELECT COUNT(*) FROM links WHERE link_source = 'explicit') AS explicit_links,
    (SELECT COUNT(*) FROM links WHERE link_source = 'relationship') AS typed_links,
    (SELECT COUNT(*) FROM links WHERE link_source = 'mention') AS mention_links,
    (SELECT COUNT(*) FROM unresolved_links) AS unresolved_links;

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
