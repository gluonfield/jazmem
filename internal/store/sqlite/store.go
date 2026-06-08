package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type IndexData struct {
	Pages      []PageRecord
	Aliases    []AliasRecord
	Links      []LinkRecord
	Unresolved []UnresolvedLinkRecord
	Chunks     []ChunkRecord
}

type PageRecord struct {
	Slug          string
	Path          string
	Type          string
	Title         string
	AliasesJSON   string
	BodyHash      string
	Frontmatter   map[string]any
	ModifiedAt    time.Time
	IndexedAt     time.Time
	ExtractorHash string
}

type AliasRecord struct {
	Slug            string
	Alias           string
	NormalizedAlias string
}

type LinkRecord struct {
	FromSlug   string
	ToSlug     string
	LinkType   string
	LinkSource string
	Display    string
	Context    string
}

type UnresolvedLinkRecord struct {
	FromSlug string
	Target   string
	Display  string
	Reason   string
	Context  string
}

type ChunkRecord struct {
	Slug       string
	Index      int
	Title      string
	Body       string
	BodyHash   string
	Embedding  []byte
	ModifiedAt time.Time
}

type SearchResult struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	ChunkIndex int     `json:"chunk_index"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

type DoctorReport struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	LinkCount       int `json:"link_count"`
	UnresolvedCount int `json:"unresolved_count"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) configure() error {
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pages (
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
		)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			slug TEXT NOT NULL,
			alias TEXT NOT NULL,
			normalized_alias TEXT NOT NULL,
			PRIMARY KEY (slug, normalized_alias)
		)`,
		`CREATE INDEX IF NOT EXISTS aliases_normalized_idx ON aliases(normalized_alias)`,
		`CREATE TABLE IF NOT EXISTS links (
			from_slug TEXT NOT NULL,
			to_slug TEXT NOT NULL,
			link_type TEXT NOT NULL,
			link_source TEXT NOT NULL,
			display TEXT NOT NULL,
			context TEXT NOT NULL,
			PRIMARY KEY (from_slug, to_slug, link_type, link_source, display, context)
		)`,
		`CREATE INDEX IF NOT EXISTS links_to_idx ON links(to_slug)`,
		`CREATE TABLE IF NOT EXISTS unresolved_links (
			from_slug TEXT NOT NULL,
			target TEXT NOT NULL,
			display TEXT NOT NULL,
			reason TEXT NOT NULL,
			context TEXT NOT NULL,
			PRIMARY KEY (from_slug, target, display, reason, context)
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			slug TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			body TEXT NOT NULL,
			body_hash TEXT NOT NULL,
			embedding BLOB,
			modified_at_ms INTEGER NOT NULL,
			PRIMARY KEY (slug, chunk_index)
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			slug UNINDEXED,
			chunk_index UNINDEXED,
			title,
			body
		)`,
		`CREATE TABLE IF NOT EXISTS scheduler_state (
			task TEXT PRIMARY KEY,
			last_run_at_ms INTEGER NOT NULL,
			last_status TEXT NOT NULL,
			last_error TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS index_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Rebuild(ctx context.Context, data IndexData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	for _, stmt := range []string{
		`DELETE FROM chunks_fts`,
		`DELETE FROM chunks`,
		`DELETE FROM unresolved_links`,
		`DELETE FROM links`,
		`DELETE FROM aliases`,
		`DELETE FROM pages`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := insertPages(ctx, tx, data.Pages); err != nil {
		return err
	}
	if err := insertAliases(ctx, tx, data.Aliases); err != nil {
		return err
	}
	if err := insertLinks(ctx, tx, data.Links); err != nil {
		return err
	}
	if err := insertUnresolved(ctx, tx, data.Unresolved); err != nil {
		return err
	}
	if err := insertChunks(ctx, tx, data.Chunks); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO index_state(key, value, updated_at_ms) VALUES('last_rebuild', ?, ?)`, time.Now().UTC().Format(time.RFC3339), millis(time.Now().UTC())); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	tokens := ftsTokens(query)
	if match := ftsQueryAll(tokens); match != "" {
		results, err := s.searchFTS(ctx, match, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil && !isFTSQuerySyntaxError(err) {
			return nil, err
		}
	}
	if match := ftsQueryAny(tokens); match != "" {
		results, err := s.searchFTS(ctx, match, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil && !isFTSQuerySyntaxError(err) {
			return nil, err
		}
	}
	return s.searchLike(ctx, query, limit)
}

func (s *Store) searchFTS(ctx context.Context, match string, limit int) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.slug, f.chunk_index, p.title,
		snippet(chunks_fts, 3, '[', ']', '...', 16) AS snippet,
		bm25(chunks_fts) AS rank
		FROM chunks_fts f
		JOIN pages p ON p.slug = f.slug
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (s *Store) searchLike(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT c.slug, c.chunk_index, p.title,
		substr(c.body, 1, 240) AS snippet,
		0.0 AS rank
		FROM chunks c
		JOIN pages p ON p.slug = c.slug
		WHERE p.title LIKE ? OR c.body LIKE ?
		ORDER BY p.title, c.chunk_index
		LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (s *Store) Doctor(ctx context.Context) (DoctorReport, error) {
	var report DoctorReport
	counts := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM pages`, &report.PageCount},
		{`SELECT COUNT(*) FROM chunks`, &report.ChunkCount},
		{`SELECT COUNT(*) FROM links`, &report.LinkCount},
		{`SELECT COUNT(*) FROM unresolved_links`, &report.UnresolvedCount},
	}
	for _, count := range counts {
		if err := s.db.QueryRowContext(ctx, count.query).Scan(count.dest); err != nil {
			return DoctorReport{}, err
		}
	}
	return report, nil
}

func (s *Store) Optimize(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chunks_fts(chunks_fts) VALUES('optimize')`)
	return err
}

func (s *Store) RecordTask(ctx context.Context, task, status string, runAt time.Time, errText string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO scheduler_state(task, last_run_at_ms, last_status, last_error)
		VALUES(?, ?, ?, ?)`, task, millis(runAt), status, errText)
	return err
}

func (s *Store) TaskState(ctx context.Context, task string) (time.Time, string, error) {
	var ms int64
	var status, errText string
	err := s.db.QueryRowContext(ctx, `SELECT last_run_at_ms, last_status, last_error FROM scheduler_state WHERE task = ?`, task).Scan(&ms, &status, &errText)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, "", nil
	}
	if err != nil {
		return time.Time{}, "", err
	}
	if errText != "" {
		status = status + ": " + errText
	}
	return time.UnixMilli(ms).UTC(), status, nil
}

func insertPages(ctx context.Context, tx *sql.Tx, pages []PageRecord) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO pages(slug, path, type, title, aliases_json, body_hash, frontmatter_json, modified_at_ms, indexed_at_ms, extractor_hash)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, page := range pages {
		fm, err := json.Marshal(page.Frontmatter)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, page.Slug, page.Path, page.Type, page.Title, page.AliasesJSON, page.BodyHash, string(fm), millis(page.ModifiedAt), millis(page.IndexedAt), page.ExtractorHash); err != nil {
			return err
		}
	}
	return nil
}

func insertAliases(ctx context.Context, tx *sql.Tx, aliases []AliasRecord) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO aliases(slug, alias, normalized_alias) VALUES(?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, alias := range aliases {
		if _, err := stmt.ExecContext(ctx, alias.Slug, alias.Alias, alias.NormalizedAlias); err != nil {
			return err
		}
	}
	return nil
}

func insertLinks(ctx context.Context, tx *sql.Tx, links []LinkRecord) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO links(from_slug, to_slug, link_type, link_source, display, context) VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, link := range links {
		if link.FromSlug == "" || link.ToSlug == "" || link.FromSlug == link.ToSlug {
			continue
		}
		if _, err := stmt.ExecContext(ctx, link.FromSlug, link.ToSlug, link.LinkType, link.LinkSource, link.Display, link.Context); err != nil {
			return err
		}
	}
	return nil
}

func insertUnresolved(ctx context.Context, tx *sql.Tx, unresolved []UnresolvedLinkRecord) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO unresolved_links(from_slug, target, display, reason, context) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, link := range unresolved {
		if _, err := stmt.ExecContext(ctx, link.FromSlug, link.Target, link.Display, link.Reason, link.Context); err != nil {
			return err
		}
	}
	return nil
}

func insertChunks(ctx context.Context, tx *sql.Tx, chunks []ChunkRecord) error {
	chunkStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks(slug, chunk_index, body, body_hash, embedding, modified_at_ms) VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer chunkStmt.Close()
	ftsStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks_fts(slug, chunk_index, title, body) VALUES(?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()
	for _, chunk := range chunks {
		if _, err := chunkStmt.ExecContext(ctx, chunk.Slug, chunk.Index, chunk.Body, chunk.BodyHash, chunk.Embedding, millis(chunk.ModifiedAt)); err != nil {
			return err
		}
		if _, err := ftsStmt.ExecContext(ctx, chunk.Slug, chunk.Index, chunk.Title, chunk.Body); err != nil {
			return err
		}
	}
	return nil
}

func scanSearchRows(rows *sql.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.Slug, &result.ChunkIndex, &result.Title, &result.Snippet, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}

var ftsToken = regexp.MustCompile(`[A-Za-z0-9_]+`)

func ftsTokens(query string) []string {
	tokens := ftsToken.FindAllString(query, -1)
	if len(tokens) > 8 {
		tokens = tokens[:8]
	}
	return tokens
}

func ftsQueryAll(tokens []string) string {
	return ftsQueryJoin(tokens, " AND ")
}

func ftsQueryAny(tokens []string) string {
	if len(tokens) <= 1 {
		return ""
	}
	return ftsQueryJoin(tokens, " OR ")
}

func ftsQueryJoin(tokens []string, separator string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ReplaceAll(token, `"`, `""`)
		parts = append(parts, fmt.Sprintf(`"%s"`, token))
	}
	return strings.Join(parts, separator)
}

func isFTSQuerySyntaxError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "fts5") &&
		(strings.Contains(text, "syntax") || strings.Contains(text, "malformed") || strings.Contains(text, "unterminated"))
}
