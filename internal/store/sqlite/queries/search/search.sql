-- name: SearchTitleAliasTerm :many
WITH candidates AS (
	SELECT p.slug,
		CASE
			WHEN lower(p.title) = ? THEN -4.00
			WHEN lower(p.title) LIKE ? THEN -3.00
			ELSE 10.00
		END AS score
	FROM pages p
	WHERE (lower(p.title) = ? OR lower(p.title) LIKE ?) AND p.slug NOT LIKE 'dreams/%'
	UNION ALL
	SELECT a.slug,
		CASE
			WHEN a.normalized_alias = ? THEN -4.20
			WHEN a.normalized_alias LIKE ? THEN -3.20
			ELSE 10.00
		END AS score
	FROM aliases a
	WHERE (a.normalized_alias = ? OR a.normalized_alias LIKE ?) AND a.slug NOT LIKE 'dreams/%'
),
ranked AS (
	SELECT slug, CAST(MIN(score) AS REAL) AS score
	FROM candidates
	GROUP BY slug
	ORDER BY score
	LIMIT ?
)
SELECT r.slug,
	COALESCE(c.chunk_index, 0) AS chunk_index,
	p.title,
	COALESCE(substr(c.body, 1, 600), p.title) AS snippet,
	r.score
FROM ranked r
JOIN pages p ON p.slug = r.slug
LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
ORDER BY r.score, p.title;

-- name: SearchLike :many
SELECT c.slug,
	c.chunk_index,
	p.title,
	substr(c.body, 1, 600) AS snippet,
	CAST(CASE WHEN p.title LIKE ? THEN -0.5 ELSE 0.0 END AS REAL) AS score
FROM chunks c
JOIN pages p ON p.slug = c.slug
WHERE (p.title LIKE ? OR c.body LIKE ?) AND c.slug NOT LIKE 'dreams/%'
ORDER BY score, p.title, c.chunk_index
LIMIT ?;
