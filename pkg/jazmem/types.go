package jazmem

import (
	"context"
	"errors"
	"time"
)

type Config struct {
	Root             string
	DBPath           string
	ProviderEndpoint string
	APIKey           string
	Model            string
	ReasoningEffort  string
	Now              func() time.Time
	DreamRunner      DreamRunner
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
	Links       []LinkRef      `json:"links,omitempty"`
	Backlinks   []LinkRef      `json:"backlinks,omitempty"`
}

// LinkRef is one edge of a page's graph neighborhood. Type is the link type
// (reference, mention, or a typed relationship such as works_at); Source is
// how it was derived (explicit, mention, relationship).
type LinkRef struct {
	Slug   string `json:"slug"`
	Type   string `json:"type"`
	Source string `json:"source"`
}

type SearchOptions struct {
	Limit int  `json:"limit,omitempty"`
	Deep  bool `json:"deep,omitempty"`
}

type AgenticOptions struct {
	Deep bool `json:"deep,omitempty"`
}

type Match struct {
	Chunk   int     `json:"chunk"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type Result struct {
	Slug  string  `json:"slug"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
	// Via is set only when it changes how the agent should treat the hit:
	// "relationship" (typed-edge match) or "link" (neighbor pulled in by
	// expansion, not a direct match). Direct text/title hits carry no via.
	Via        string    `json:"via,omitempty"`
	ModifiedAt time.Time `json:"modified_at,omitzero"`
	Matches    []Match   `json:"matches"`
}

type SearchStats struct {
	Pages     int `json:"pages"`
	Chunks    int `json:"chunks"`
	GraphHits int `json:"graph_hits,omitempty"`
}

type SearchResponse struct {
	Results  []Result    `json:"results"`
	Stats    SearchStats `json:"stats"`
	Warnings []string    `json:"warnings,omitempty"`
}

// Citation links an answer's [n] markers back to evidence: ID is the marker
// number used in the answer text.
type Citation struct {
	ID    int    `json:"id,omitempty"`
	Slug  string `json:"slug"`
	Title string `json:"title,omitempty"`
	Chunk int    `json:"chunk"`
}

type AgenticResponse struct {
	Answer      string         `json:"answer"`
	Citations   []Citation     `json:"citations"`
	Gaps        []string       `json:"gaps,omitempty"`
	Stats       SearchStats    `json:"stats"`
	Warnings    []string       `json:"warnings,omitempty"`
	ModelUsed   string         `json:"model_used,omitempty"`
	Rounds      int            `json:"rounds"`
	SynthesisOK bool           `json:"synthesis_ok"`
	Diagnostics map[string]int `json:"diagnostics,omitempty"`
	Results     []Result       `json:"results,omitempty"`
}

type ReindexOptions struct{}

type Report struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	ExplicitLinks   int `json:"explicit_links"`
	TypedLinks      int `json:"typed_links"`
	MentionLinks    int `json:"mention_links"`
	UnresolvedLinks int `json:"unresolved_links"`
}

type DreamOptions struct {
	Date time.Time `json:"date,omitzero"`
}

var ErrDreamRunnerUnavailable = errors.New("dream runner unavailable")

// DreamRunner lets an embedding host replace jazmem's provider-backed dream
// prompt with a richer runtime, such as a coding agent that edits markdown
// directly. jazmem still owns indexing before and after the run.
type DreamRunner interface {
	RunDream(context.Context, DreamRequest) (DreamReport, error)
}

type DreamRequest struct {
	Root   string    `json:"root"`
	DBPath string    `json:"db_path"`
	Date   time.Time `json:"date"`
}

type DreamReport struct {
	RunSlug          string   `json:"run_slug"`
	ReviewSlug       string   `json:"review_slug,omitempty"`
	InputSlugs       []string `json:"input_slugs"`
	Promoted         int      `json:"promoted"`
	ReviewItems      int      `json:"review_items"`
	Skipped          int      `json:"skipped"`
	LongTermUpdated  bool     `json:"long_term_updated,omitempty"`
	ShortTermUpdated bool     `json:"short_term_updated,omitempty"`
	TasksArchived    int      `json:"tasks_archived,omitempty"`
	ModelUsed        string   `json:"model_used,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type DreamTaskReport struct {
	Index Report      `json:"index"`
	Dream DreamReport `json:"dream"`
}

type EvalCase struct {
	Query         string   `json:"query"`
	ExpectedSlugs []string `json:"expected_slugs"`
	Limit         int      `json:"limit,omitempty"`
}

type EvalOptions struct {
	Cases []EvalCase `json:"cases,omitempty"`
	Limit int        `json:"limit,omitempty"`
}

type EvalCaseResult struct {
	Query          string   `json:"query"`
	ExpectedSlugs  []string `json:"expected_slugs"`
	ReturnedSlugs  []string `json:"returned_slugs"`
	Hits           int      `json:"hits"`
	Precision      float64  `json:"precision"`
	Recall         float64  `json:"recall"`
	ReciprocalRank float64  `json:"reciprocal_rank"`
}

type EvalReport struct {
	CaseCount   int              `json:"case_count"`
	Limit       int              `json:"limit"`
	HitRate     float64          `json:"hit_rate"`
	Precision   float64          `json:"precision"`
	Recall      float64          `json:"recall"`
	MRR         float64          `json:"mrr"`
	CaseResults []EvalCaseResult `json:"cases"`
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
	TypedLinkCount  int    `json:"typed_link_count"`
	UnresolvedCount int    `json:"unresolved_count"`
}
