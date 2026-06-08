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
	"sort"
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
	TypedLinkCount  int `json:"typed_link_count"`
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
		`CREATE INDEX IF NOT EXISTS links_type_idx ON links(link_type, from_slug, to_slug)`,
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
	limit = normalizeLimit(limit)
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

func (s *Store) SearchTitleAlias(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	terms := lookupTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	bySlug := map[string]SearchResult{}
	for _, term := range terms {
		rows, err := s.searchTitleAliasTerm(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			current, ok := bySlug[row.Slug]
			if !ok || row.Score < current.Score {
				bySlug[row.Slug] = row
			}
		}
	}
	results := make([]SearchResult, 0, len(bySlug))
	for _, row := range bySlug {
		results = append(results, row)
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *Store) LinkedPages(ctx context.Context, seeds []string, limit int) ([]SearchResult, error) {
	seeds = uniqueNonEmpty(seeds)
	if len(seeds) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	values := make([]string, 0, len(seeds))
	args := make([]any, 0, len(seeds)+1)
	for _, seed := range seeds {
		values = append(values, "(?)")
		args = append(args, seed)
	}
	args = append(args, limit)
	query := fmt.Sprintf(`WITH seed(slug) AS (VALUES %s),
		edges AS (
			SELECT l.to_slug AS slug,
				CASE WHEN l.link_source = 'explicit' THEN 0.80 ELSE 1.20 END AS score
			FROM links l
			JOIN seed ON seed.slug = l.from_slug
			UNION ALL
			SELECT l.from_slug AS slug,
				CASE WHEN l.link_source = 'explicit' THEN 0.90 ELSE 1.30 END AS score
			FROM links l
			JOIN seed ON seed.slug = l.to_slug
		),
		ranked AS (
			SELECT slug, MIN(score) AS score
			FROM edges
			WHERE slug NOT IN (SELECT slug FROM seed)
			GROUP BY slug
			ORDER BY score
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			COALESCE(substr(c.body, 1, 600), p.title) AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`, strings.Join(values, ","))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (s *Store) ResolveEntity(ctx context.Context, text string) (string, error) {
	text = cleanEntityPhrase(text)
	if text == "" {
		return "", nil
	}
	slug := cleanSlug(text)
	if slug != "" {
		var found string
		err := s.db.QueryRowContext(ctx, `SELECT slug FROM pages WHERE slug = ?`, slug).Scan(&found)
		if err == nil {
			return found, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	normalized := normalizeEntity(text)
	rows, err := s.db.QueryContext(ctx, `SELECT slug FROM (
			SELECT slug FROM aliases WHERE normalized_alias = ?
			UNION
			SELECT slug FROM pages WHERE lower(title) = ?
		)
		ORDER BY slug
		LIMIT 2`, normalized, normalized)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var matches []string
	for rows.Next() {
		var match string
		if err := rows.Scan(&match); err != nil {
			return "", err
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", nil
	}
	return matches[0], nil
}

func (s *Store) RelationalFanout(ctx context.Context, seed string, linkTypes []string, direction string, limit int) ([]SearchResult, error) {
	seed = strings.TrimSpace(seed)
	linkTypes = uniqueNonEmpty(linkTypes)
	if seed == "" || len(linkTypes) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	typeWhere, args := linkTypeWhere(linkTypes)
	var edgeSQL string
	switch direction {
	case "out":
		args = append(args, seed)
		edgeSQL = `SELECT to_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND from_slug = ?`
	case "both":
		args = append(args, seed)
		args = append(args, stringSliceToAny(linkTypes)...)
		args = append(args, seed)
		edgeSQL = `SELECT to_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND from_slug = ?
			UNION ALL
			SELECT from_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND to_slug = ?`
	default:
		args = append(args, seed)
		edgeSQL = `SELECT from_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND to_slug = ?`
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `WITH edges AS (`+edgeSQL+`),
		ranked AS (
			SELECT slug, MIN(score) AS score, MIN(link_type) AS link_type, MIN(context) AS context
			FROM edges
			WHERE slug <> ''
			GROUP BY slug
			ORDER BY score, slug
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			CASE
				WHEN r.context <> '' THEN '[' || r.link_type || '] ' || r.context
				ELSE COALESCE(substr(c.body, 1, 600), p.title)
			END AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (s *Store) RelationalBetween(ctx context.Context, left, right string, limit int) ([]SearchResult, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" || left == right {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	rows, err := s.db.QueryContext(ctx, `WITH undirected AS (
			SELECT from_slug AS a, to_slug AS b, link_type, context
			FROM links
			WHERE link_source = 'relationship'
			UNION ALL
			SELECT to_slug AS a, from_slug AS b, link_type, context
			FROM links
			WHERE link_source = 'relationship'
		),
		candidates AS (
			SELECT ? AS slug, -20.0 AS score, link_type, context
			FROM undirected
			WHERE a = ? AND b = ?
			UNION ALL
			SELECT ? AS slug, -20.0 AS score, link_type, context
			FROM undirected
			WHERE a = ? AND b = ?
			UNION ALL
			SELECT u1.b AS slug, -19.0 AS score, u1.link_type || '+' || u2.link_type AS link_type,
				COALESCE(NULLIF(u1.context, ''), '') || CASE WHEN u1.context <> '' AND u2.context <> '' THEN ' / ' ELSE '' END || COALESCE(NULLIF(u2.context, ''), '') AS context
			FROM undirected u1
			JOIN undirected u2 ON u2.b = u1.b
			WHERE u1.a = ? AND u2.a = ? AND u1.b NOT IN (?, ?)
		),
		ranked AS (
			SELECT slug, MIN(score) AS score, MIN(link_type) AS link_type, MIN(context) AS context
			FROM candidates
			WHERE slug <> ''
			GROUP BY slug
			ORDER BY score, slug
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			CASE
				WHEN r.context <> '' THEN '[' || r.link_type || '] ' || r.context
				ELSE COALESCE(substr(c.body, 1, 600), p.title)
			END AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`,
		left, left, right,
		right, right, left,
		left, right, left, right,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchRows(rows)
}

func (s *Store) searchFTS(ctx context.Context, match string, limit int) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.slug, f.chunk_index, p.title,
		substr(f.body, 1, 240) AS snippet,
		bm25(chunks_fts) AS rank
		FROM chunks_fts f
		JOIN pages p ON p.slug = f.slug
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, match, chunkPoolLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results, err := scanSearchRows(rows)
	if err != nil {
		return nil, err
	}
	return bestPerPage(results, limit), nil
}

func (s *Store) searchTitleAliasTerm(ctx context.Context, term string, limit int) ([]SearchResult, error) {
	like := "%" + term + "%"
	rows, err := s.db.QueryContext(ctx, `WITH candidates AS (
			SELECT p.slug,
				CASE
					WHEN lower(p.title) = ? THEN -4.00
					WHEN lower(p.title) LIKE ? THEN -3.00
					ELSE 10.00
				END AS score
			FROM pages p
			WHERE lower(p.title) = ? OR lower(p.title) LIKE ?
			UNION ALL
			SELECT a.slug,
				CASE
					WHEN a.normalized_alias = ? THEN -4.20
					WHEN a.normalized_alias LIKE ? THEN -3.20
					ELSE 10.00
				END AS score
			FROM aliases a
			WHERE a.normalized_alias = ? OR a.normalized_alias LIKE ?
		),
		ranked AS (
			SELECT slug, MIN(score) AS score
			FROM candidates
			GROUP BY slug
			ORDER BY score
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			COALESCE(substr(c.body, 1, 600), p.title) AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`,
		term, like, term, like,
		term, like, term, like,
		limit)
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
		CASE WHEN p.title LIKE ? THEN -0.5 ELSE 0.0 END AS rank
		FROM chunks c
		JOIN pages p ON p.slug = c.slug
		WHERE p.title LIKE ? OR c.body LIKE ?
		ORDER BY rank, p.title, c.chunk_index
		LIMIT ?`, like, like, like, chunkPoolLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results, err := scanSearchRows(rows)
	if err != nil {
		return nil, err
	}
	return bestPerPage(results, limit), nil
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
		{`SELECT COUNT(*) FROM links WHERE link_source = 'relationship'`, &report.TypedLinkCount},
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

var searchStopwords = map[string]bool{
	"about": true,
	"and":   true,
	"are":   true,
	"for":   true,
	"how":   true,
	"the":   true,
	"what":  true,
	"where": true,
	"which": true,
	"who":   true,
	"why":   true,
	"with":  true,
	"from":  true,
	"into":  true,
	"is":    true,
	"me":    true,
	"my":    true,
	"tell":  true,
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func chunkPoolLimit(pageLimit int) int {
	limit := normalizeLimit(pageLimit) * 8
	if limit < 50 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func ftsTokens(query string) []string {
	tokens := ftsToken.FindAllString(query, -1)
	if len(tokens) > 8 {
		tokens = tokens[:8]
	}
	return tokens
}

func lookupTerms(query string) []string {
	full := normalizeLookup(query)
	tokens := ftsTokens(query)
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		term = normalizeLookup(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	add(full)
	for _, token := range tokens {
		term := normalizeLookup(token)
		if searchStopwords[term] {
			continue
		}
		if len(term) < 2 && len(tokens) > 1 {
			continue
		}
		add(term)
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

func normalizeLookup(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func normalizeEntity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.Join(strings.Fields(s), " ")
}

func cleanEntityPhrase(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(strings.ToLower(s), prefix) && len(s) > len(prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}
	return s
}

func cleanSlug(s string) string {
	s = filepath.ToSlash(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".md")
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	s = strings.Trim(s, "/")
	if strings.Contains(s, "..") {
		return ""
	}
	return strings.ToLower(s)
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

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sortSearchResults(results []SearchResult) {
	sort.Slice(results, func(a, b int) bool {
		if results[a].Score == results[b].Score {
			return results[a].Slug < results[b].Slug
		}
		return results[a].Score < results[b].Score
	})
}

func bestPerPage(rows []SearchResult, limit int) []SearchResult {
	limit = normalizeLimit(limit)
	bySlug := map[string]SearchResult{}
	for _, row := range rows {
		current, ok := bySlug[row.Slug]
		if !ok || row.Score < current.Score || (row.Score == current.Score && row.ChunkIndex < current.ChunkIndex) {
			bySlug[row.Slug] = row
		}
	}
	results := make([]SearchResult, 0, len(bySlug))
	for _, row := range bySlug {
		results = append(results, row)
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func linkTypeWhere(linkTypes []string) (string, []any) {
	if len(linkTypes) == 1 {
		return "link_type = ?", []any{linkTypes[0]}
	}
	parts := make([]string, 0, len(linkTypes))
	args := make([]any, 0, len(linkTypes))
	for _, linkType := range linkTypes {
		parts = append(parts, "?")
		args = append(args, linkType)
	}
	return "link_type IN (" + strings.Join(parts, ",") + ")", args
}

func stringSliceToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}
