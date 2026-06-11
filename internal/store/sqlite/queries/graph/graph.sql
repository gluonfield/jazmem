-- name: OutgoingLinks :many
SELECT DISTINCT to_slug, link_type, link_source
FROM links
WHERE from_slug = ?
ORDER BY to_slug, link_type, link_source;

-- name: IncomingLinks :many
SELECT DISTINCT from_slug, link_type, link_source
FROM links
WHERE to_slug = ?
ORDER BY from_slug, link_type, link_source;
