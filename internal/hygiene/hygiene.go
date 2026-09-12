package hygiene

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/indexer"
	"github.com/gluonfield/jazmem/internal/memfs"
	"github.com/gluonfield/jazmem/internal/mention"
)

type Service struct {
	FS  *memfs.FileSystem
	Now func() time.Time
}

type Report struct {
	RelationshipsAdded int      `json:"relationships_added"`
	ProposalCount      int      `json:"proposal_count"`
	ReviewSlug         string   `json:"review_slug,omitempty"`
	PagesChanged       []string `json:"pages_changed"`
	Proposals          []Proposal
}

type Proposal struct {
	FromSlug           string `json:"from_slug"`
	ToSlug             string `json:"to_slug"`
	Label              string `json:"label"`
	SourceSlug         string `json:"source_slug"`
	Reason             string `json:"reason"`
	ForwardMarkdown    string `json:"forward_markdown"`
	ReciprocalMarkdown string `json:"reciprocal_markdown"`
}

type entity struct {
	Page    memfs.Page
	Aliases []string
}

type relationship struct {
	From   memfs.Page
	To     memfs.Page
	Label  string
	Source memfs.Page
}

func (s *Service) Run(ctx context.Context) (Report, error) {
	pages, err := s.FS.ListPages(ctx)
	if err != nil {
		return Report{}, err
	}
	relationships, err := discoverRelationships(ctx, pages)
	if err != nil {
		return Report{}, err
	}
	if len(relationships) == 0 {
		return Report{}, nil
	}
	proposals := make([]Proposal, 0, len(relationships))
	for _, rel := range relationships {
		if hasRelationship(rel.From.Raw, rel.To.Slug) && hasRelationship(rel.To.Raw, rel.From.Slug) {
			continue
		}
		proposals = append(proposals, s.proposal(rel))
	}
	if len(proposals) == 0 {
		return Report{}, nil
	}
	reviewSlug, err := s.writeReview(proposals)
	if err != nil {
		return Report{}, err
	}
	return Report{
		RelationshipsAdded: 0,
		ProposalCount:      len(proposals),
		ReviewSlug:         reviewSlug,
		PagesChanged:       []string{reviewSlug},
		Proposals:          proposals,
	}, nil
}

func (s *Service) proposal(rel relationship) Proposal {
	date := s.now().Local().Format("2006-01-02")
	forward := relationshipBullet(rel.To.Slug, rel.Label, rel.Source.Slug, date)
	reciprocal := relationshipBullet(rel.From.Slug, rel.Label, rel.Source.Slug, date)
	return Proposal{
		FromSlug:           rel.From.Slug,
		ToSlug:             rel.To.Slug,
		Label:              rel.Label,
		SourceSlug:         rel.Source.Slug,
		Reason:             "heuristic relationship mention; review before promoting to canonical pages",
		ForwardMarkdown:    forward,
		ReciprocalMarkdown: reciprocal,
	}
}

func (s *Service) writeReview(proposals []Proposal) (string, error) {
	date := s.now().Local().Format("2006-01-02")
	slug := "dreams/review/link-hygiene-" + date
	var b strings.Builder
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Link Hygiene Review " + date,
		"type":  "review",
		"date":  date,
	}))
	fmt.Fprintf(&b, "# Link Hygiene Review %s\n\n", date)
	b.WriteString("These are derived relationship proposals. Promote them by editing the canonical markdown pages directly; Jaz owns indexing and maintenance.\n\n")
	for i, proposal := range proposals {
		fmt.Fprintf(&b, "## Proposal %d\n\n", i+1)
		fmt.Fprintf(&b, "- From: [[%s]]\n", proposal.FromSlug)
		fmt.Fprintf(&b, "- To: [[%s]]\n", proposal.ToSlug)
		fmt.Fprintf(&b, "- Source: [[%s]]\n", proposal.SourceSlug)
		fmt.Fprintf(&b, "- Label: %s\n", proposal.Label)
		fmt.Fprintf(&b, "- Reason: %s\n\n", proposal.Reason)
		b.WriteString("Forward:\n\n")
		fmt.Fprintf(&b, "```md\n%s\n```\n\n", proposal.ForwardMarkdown)
		b.WriteString("Reciprocal:\n\n")
		fmt.Fprintf(&b, "```md\n%s\n```\n\n", proposal.ReciprocalMarkdown)
	}
	return slug, s.FS.WritePage(slug, b.String())
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func discoverRelationships(ctx context.Context, pages []memfs.Page) ([]relationship, error) {
	entities := entitiesFromPages(pages)
	var aliases []string
	var owners []int
	bySlug := map[string]int{}
	for index, entity := range entities {
		bySlug[entity.Page.Slug] = index
		for _, alias := range entity.Aliases {
			aliases = append(aliases, alias)
			owners = append(owners, index)
		}
	}
	matcher := mention.New(aliases)
	var out []relationship
	seen := map[string]bool{}
	for _, source := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(indexer.StripCode(source.Body), "\n") {
			label := relationshipLabel(line)
			if label == "" {
				continue
			}
			matches, err := matcher.Find(ctx, line)
			if err != nil {
				return nil, err
			}
			mentioned := map[int]bool{}
			if owner, ok := bySlug[source.Slug]; ok {
				mentioned[owner] = true
			}
			for _, match := range matches {
				mentioned[owners[match.Alias]] = true
			}
			if len(mentioned) != 2 {
				continue
			}
			var pair []int
			for owner := range mentioned {
				pair = append(pair, owner)
			}
			sort.Ints(pair)
			left, right := entities[pair[0]].Page, entities[pair[1]].Page
			key := left.Slug + "\x00" + right.Slug + "\x00" + label
			if !seen[key] {
				seen[key] = true
				out = append(out, relationship{From: left, To: right, Label: label, Source: source})
			}
		}
	}
	return out, nil
}

func entitiesFromPages(pages []memfs.Page) []entity {
	var out []entity
	for _, page := range pages {
		if !isEntity(page) {
			continue
		}
		out = append(out, entity{Page: page, Aliases: relationshipAliases(page)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Page.Slug < out[j].Page.Slug })
	return out
}

func isEntity(page memfs.Page) bool {
	switch page.Type {
	case "people", "companies", "projects":
		return true
	default:
		return strings.HasPrefix(page.Slug, "people/") || strings.HasPrefix(page.Slug, "companies/") || strings.HasPrefix(page.Slug, "projects/")
	}
}

func relationshipAliases(page memfs.Page) []string {
	values := []string{page.Title, strings.ReplaceAll(slugTail(page.Slug), "-", " ")}
	values = append(values, page.Aliases...)
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func relationshipLabel(body string) string {
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "friends"), strings.Contains(lower, "friend"):
		return "friend"
	case strings.Contains(lower, "works with"), strings.Contains(lower, "working with"):
		return "works with"
	case strings.Contains(lower, "knows"), strings.Contains(lower, "know each other"):
		return "knows"
	default:
		return ""
	}
}

func hasRelationship(raw, targetSlug string) bool {
	section := relationshipSection(raw)
	return strings.Contains(section, "[["+targetSlug+"]]")
}

func relationshipBullet(targetSlug, label, sourceSlug, date string) string {
	return fmt.Sprintf("- [[%s]] - %s. [Source: [[%s]], %s]", targetSlug, label, sourceSlug, date)
}

func relationshipSection(raw string) string {
	start, end := relationshipSectionBounds(raw)
	if start < 0 {
		return ""
	}
	return raw[start:end]
}

func relationshipSectionBounds(raw string) (int, int) {
	lines := strings.SplitAfter(raw, "\n")
	offset := 0
	start := -1
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if trimmed == "## Relationships" {
				start = offset + len(line)
			} else if start >= 0 {
				return start, offset
			}
		}
		offset += len(line)
	}
	if start >= 0 {
		return start, len(raw)
	}
	return -1, -1
}

func slugTail(slug string) string {
	parts := strings.Split(slug, "/")
	return parts[len(parts)-1]
}
