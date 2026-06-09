-- name: CountPages :one
SELECT COUNT(*) FROM pages;

-- name: CountChunks :one
SELECT COUNT(*) FROM chunks;

-- name: CountLinks :one
SELECT COUNT(*) FROM links;

-- name: CountRelationshipLinks :one
SELECT COUNT(*) FROM links
WHERE link_source = 'relationship';

-- name: CountUnresolvedLinks :one
SELECT COUNT(*) FROM unresolved_links;

-- name: OptimizeFTS :exec
INSERT INTO chunks_fts(chunks_fts)
VALUES ('optimize');

-- name: RecordTask :exec
INSERT OR REPLACE INTO scheduler_state(task, last_run_at_ms, last_status, last_error)
VALUES (?, ?, ?, ?);

-- name: GetTaskState :one
SELECT last_run_at_ms, last_status, last_error
FROM scheduler_state
WHERE task = ?;
