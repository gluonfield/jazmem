-- name: FindPageSlug :one
SELECT slug
FROM pages
WHERE slug = ?;

-- name: ResolveEntitySlugs :many
SELECT slug
FROM (
	SELECT slug
	FROM aliases
	WHERE normalized_alias = ?
	UNION
	SELECT slug
	FROM pages
	WHERE lower(title) = ?
)
ORDER BY slug
LIMIT 2;
