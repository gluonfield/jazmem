package jazmem

import (
	"context"

	"github.com/wins/jazmem/internal/dream"
	"github.com/wins/jazmem/internal/memfs"
)

func (m *Memory) GetPage(ctx context.Context, slug string) (Page, error) {
	select {
	case <-ctx.Done():
		return Page{}, ctx.Err()
	default:
	}
	page, err := m.fs.ReadPage(slug)
	if err != nil {
		return Page{}, m.notFoundError(slug, err)
	}
	return publicPage(page), nil
}

func (m *Memory) Reindex(ctx context.Context, _ ReindexOptions) (Report, error) {
	report, err := m.indexer.Reindex(ctx)
	if err != nil {
		return Report{}, err
	}
	return Report{
		PageCount:       report.PageCount,
		ChunkCount:      report.ChunkCount,
		ExplicitLinks:   report.ExplicitLinks,
		TypedLinks:      report.TypedLinks,
		MentionLinks:    report.MentionLinks,
		UnresolvedLinks: report.UnresolvedLinks,
	}, nil
}

func (m *Memory) Dream(ctx context.Context, opts DreamOptions) (DreamReport, error) {
	report, err := m.dream.Run(ctx, dream.Options{Date: opts.Date})
	if err != nil {
		return DreamReport{}, err
	}
	return DreamReport{
		RunSlug:     report.RunSlug,
		InputSlugs:  report.InputSlugs,
		Promoted:    report.Promoted,
		ReviewItems: report.ReviewItems,
		Skipped:     report.Skipped,
	}, nil
}

func (m *Memory) LinkHygiene(ctx context.Context) (LinkHygieneReport, error) {
	report, err := m.hygiene.Run(ctx)
	if err != nil {
		return LinkHygieneReport{}, err
	}
	proposals := make([]RelationshipProposal, 0, len(report.Proposals))
	for _, proposal := range report.Proposals {
		proposals = append(proposals, RelationshipProposal{
			FromSlug:           proposal.FromSlug,
			ToSlug:             proposal.ToSlug,
			Label:              proposal.Label,
			SourceSlug:         proposal.SourceSlug,
			Reason:             proposal.Reason,
			ForwardMarkdown:    proposal.ForwardMarkdown,
			ReciprocalMarkdown: proposal.ReciprocalMarkdown,
		})
	}
	return LinkHygieneReport{
		RelationshipsAdded: report.RelationshipsAdded,
		ProposalCount:      report.ProposalCount,
		ReviewSlug:         report.ReviewSlug,
		PagesChanged:       report.PagesChanged,
		Proposals:          proposals,
	}, nil
}

func (m *Memory) Doctor(ctx context.Context) (DoctorReport, error) {
	report, err := m.store.Doctor(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	return DoctorReport{
		Root:            m.root,
		DBPath:          m.dbPath,
		PageCount:       report.PageCount,
		ChunkCount:      report.ChunkCount,
		LinkCount:       report.LinkCount,
		TypedLinkCount:  report.TypedLinkCount,
		UnresolvedCount: report.UnresolvedCount,
	}, nil
}

func publicPage(page memfs.Page) Page {
	return Page{
		Slug:        page.Slug,
		Path:        page.AbsPath,
		Type:        page.Type,
		Title:       page.Title,
		Aliases:     page.Aliases,
		Frontmatter: page.Frontmatter,
		Body:        page.Body,
		Raw:         page.Raw,
		ModifiedAt:  page.ModifiedAt,
	}
}
