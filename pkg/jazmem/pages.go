package jazmem

import (
	"context"
	"errors"

	"github.com/gluonfield/jazmem/internal/dream"
	"github.com/gluonfield/jazmem/internal/memfs"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
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
	out := publicPage(page)
	// Graph neighborhood comes from the index; a stale or empty index should
	// not make the page itself unreadable.
	if outgoing, incoming, err := m.store.PageLinks(ctx, out.Slug); err == nil {
		out.Links = publicLinkRefs(outgoing)
		out.Backlinks = publicLinkRefs(incoming)
	}
	return out, nil
}

func publicLinkRefs(refs []sqlitestore.LinkRef) []LinkRef {
	out := make([]LinkRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, LinkRef{Slug: ref.Slug, Type: ref.Type, Source: ref.Source})
	}
	return out
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
	report, err := m.runDream(ctx, opts)
	if err != nil {
		return report, err
	}
	// Task archival is deterministic, runs for both the configured-runner and
	// internal dream paths, and never deletes a task — it only rolls a residue
	// onto the linked project page.
	date := opts.Date
	if date.IsZero() {
		date = m.timeNow()
	}
	archived, warnings, archiveErr := m.dream.ArchiveDoneTasks(ctx, date.Local())
	if archiveErr != nil {
		return report, archiveErr
	}
	report.TasksArchived = archived
	report.Warnings = append(report.Warnings, warnings...)
	if archived > 0 {
		if _, err := m.Reindex(ctx, ReindexOptions{}); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (m *Memory) runDream(ctx context.Context, opts DreamOptions) (DreamReport, error) {
	if m.dreamRun != nil {
		return m.runConfiguredDream(ctx, opts)
	}
	if m.noProviderDreams {
		return DreamReport{}, errors.New("dream runner is not configured")
	}
	report, err := m.dream.Run(ctx, dream.Options{Date: opts.Date})
	if err != nil {
		return DreamReport{}, err
	}
	return DreamReport{
		RunSlug:          report.RunSlug,
		ReviewSlug:       report.ReviewSlug,
		InputSlugs:       report.InputSlugs,
		Promoted:         report.Promoted,
		ReviewItems:      report.ReviewItems,
		Skipped:          report.Skipped,
		LongTermUpdated:  report.LongTermUpdated,
		ShortTermUpdated: report.ShortTermUpdated,
		ModelUsed:        report.ModelUsed,
		Warnings:         report.Warnings,
	}, nil
}

func (m *Memory) runConfiguredDream(ctx context.Context, opts DreamOptions) (DreamReport, error) {
	if _, err := m.Reindex(ctx, ReindexOptions{}); err != nil {
		return DreamReport{}, err
	}
	date := opts.Date
	if date.IsZero() {
		date = m.timeNow()
	}
	report, err := m.dreamRun.RunDream(ctx, DreamRequest{
		Root:   m.root,
		DBPath: m.dbPath,
		Date:   date.Local(),
	})
	horizonErr := m.validateHorizonFiles()
	_, reindexErr := m.Reindex(ctx, ReindexOptions{})
	if err != nil {
		return report, errors.Join(err, reindexErr)
	}
	if horizonErr != nil {
		return report, errors.Join(horizonErr, reindexErr)
	}
	if reindexErr != nil {
		return report, reindexErr
	}
	return report, nil
}

func (m *Memory) validateHorizonFiles() error {
	for _, name := range memfs.HorizonFiles() {
		content, err := m.fs.ReadRootFile(name)
		if err != nil {
			return err
		}
		if err := memfs.ValidateHorizonContent(name, content); err != nil {
			return err
		}
	}
	return nil
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
