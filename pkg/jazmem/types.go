package jazmem

import "time"

type Config struct {
	Root   string
	DBPath string
	Now    func() time.Time
}

type PageRef struct {
	Slug string `json:"slug"`
	Path string `json:"path"`
}

type Page struct {
	Slug        string         `json:"slug"`
	Path        string         `json:"path"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Aliases     []string       `json:"aliases"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
	Raw         string         `json:"raw"`
	ModifiedAt  time.Time      `json:"modified_at"`
}

type SearchOptions struct {
	Limit int `json:"limit,omitempty"`
}

type Result struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	ChunkIndex int     `json:"chunk_index"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

type Citation struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	Path       string  `json:"path"`
	ChunkIndex int     `json:"chunk_index"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

type RetrievalDiagnostics struct {
	PagesFromBM25  int    `json:"pages_from_bm25"`
	ChunksFromBM25 int    `json:"chunks_from_bm25"`
	Mode           string `json:"mode"`
}

type RetrievalContext struct {
	Query          string               `json:"query"`
	Context        string               `json:"context"`
	Citations      []Citation           `json:"citations"`
	PagesGathered  int                  `json:"pages_gathered"`
	ChunksGathered int                  `json:"chunks_gathered"`
	Warnings       []string             `json:"warnings"`
	Diagnostics    RetrievalDiagnostics `json:"diagnostics"`
	Results        []Result             `json:"results"`
}

// SearchContext is kept as a compatibility alias. It is a retrieval envelope:
// no answer synthesis, gap analysis, or chat-model call has run.
type SearchContext = RetrievalContext

type ReindexOptions struct{}

type Report struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	ExplicitLinks   int `json:"explicit_links"`
	MentionLinks    int `json:"mention_links"`
	UnresolvedLinks int `json:"unresolved_links"`
}

type GitReport struct {
	RepoPath         string `json:"repo_path"`
	Initialized      bool   `json:"initialized"`
	GitignoreUpdated bool   `json:"gitignore_updated"`
}

type CheckpointReport struct {
	RepoPath   string `json:"repo_path"`
	Committed  bool   `json:"committed"`
	Commit     string `json:"commit,omitempty"`
	Message    string `json:"message"`
	FilesAdded int    `json:"files_added"`
}

type DreamOptions struct {
	Date time.Time `json:"date,omitempty"`
}

type DreamReport struct {
	RunSlug     string   `json:"run_slug"`
	InputSlugs  []string `json:"input_slugs"`
	Promoted    int      `json:"promoted"`
	ReviewItems int      `json:"review_items"`
	Skipped     int      `json:"skipped"`
}

type RelationshipProposal struct {
	FromSlug           string `json:"from_slug"`
	ToSlug             string `json:"to_slug"`
	Label              string `json:"label"`
	SourceSlug         string `json:"source_slug"`
	Reason             string `json:"reason"`
	ForwardMarkdown    string `json:"forward_markdown"`
	ReciprocalMarkdown string `json:"reciprocal_markdown"`
}

type LinkHygieneReport struct {
	RelationshipsAdded int                    `json:"relationships_added"`
	ProposalCount      int                    `json:"proposal_count"`
	ReviewSlug         string                 `json:"review_slug,omitempty"`
	PagesChanged       []string               `json:"pages_changed"`
	Proposals          []RelationshipProposal `json:"proposals"`
}

type DoctorReport struct {
	Root            string `json:"root"`
	DBPath          string `json:"db_path"`
	PageCount       int    `json:"page_count"`
	ChunkCount      int    `json:"chunk_count"`
	LinkCount       int    `json:"link_count"`
	UnresolvedCount int    `json:"unresolved_count"`
}
