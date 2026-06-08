package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wins/jazmem/internal/memfs"
	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

const extractorHash = "jazmem-indexer-v1"

type Indexer struct {
	FS    *memfs.FileSystem
	Store *sqlitestore.Store
}

type Report struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	ExplicitLinks   int `json:"explicit_links"`
	MentionLinks    int `json:"mention_links"`
	UnresolvedLinks int `json:"unresolved_links"`
}

type ExplicitLink struct {
	Target  string
	Display string
	Context string
}

func (i *Indexer) Reindex(ctx context.Context) (Report, error) {
	pages, err := i.FS.ListPages()
	if err != nil {
		return Report{}, err
	}
	data, report, err := buildIndex(pages)
	if err != nil {
		return Report{}, err
	}
	if err := i.Store.Rebuild(ctx, data); err != nil {
		return Report{}, err
	}
	return report, nil
}

func buildIndex(pages []memfs.Page) (sqlitestore.IndexData, Report, error) {
	now := time.Now().UTC()
	slugSet := map[string]bool{}
	for _, page := range pages {
		slugSet[page.Slug] = true
	}
	resolver := newResolver(pages)
	gazetteer := buildGazetteer(pages)

	var data sqlitestore.IndexData
	var report Report
	for _, page := range pages {
		aliasesJSON, err := json.Marshal(page.Aliases)
		if err != nil {
			return sqlitestore.IndexData{}, Report{}, err
		}
		data.Pages = append(data.Pages, sqlitestore.PageRecord{
			Slug:          page.Slug,
			Path:          page.RelPath,
			Type:          page.Type,
			Title:         page.Title,
			AliasesJSON:   string(aliasesJSON),
			BodyHash:      page.BodyHash,
			Frontmatter:   page.Frontmatter,
			ModifiedAt:    page.ModifiedAt,
			IndexedAt:     now,
			ExtractorHash: extractorHash,
		})
		for _, alias := range aliasesForPage(page) {
			data.Aliases = append(data.Aliases, sqlitestore.AliasRecord{
				Slug:            page.Slug,
				Alias:           alias,
				NormalizedAlias: NormalizeAlias(alias),
			})
		}

		clean := StripCode(page.Body)
		for _, link := range ExtractExplicitLinks(clean) {
			target, reason := resolver.Resolve(link.Target)
			if target == "" {
				data.Unresolved = append(data.Unresolved, sqlitestore.UnresolvedLinkRecord{
					FromSlug: page.Slug,
					Target:   memfs.CleanSlug(link.Target),
					Display:  link.Display,
					Reason:   reason,
					Context:  link.Context,
				})
				report.UnresolvedLinks++
				continue
			}
			if target == page.Slug {
				continue
			}
			data.Links = append(data.Links, sqlitestore.LinkRecord{
				FromSlug:   page.Slug,
				ToSlug:     target,
				LinkType:   "reference",
				LinkSource: "explicit",
				Display:    link.Display,
				Context:    link.Context,
			})
			report.ExplicitLinks++
		}

		for _, mention := range detectMentions(page, clean, gazetteer) {
			if !slugSet[mention.ToSlug] || mention.ToSlug == page.Slug {
				continue
			}
			data.Links = append(data.Links, mention)
			report.MentionLinks++
		}

		chunks := SplitChunks(page)
		for _, chunk := range chunks {
			data.Chunks = append(data.Chunks, chunk)
		}
		report.ChunkCount += len(chunks)
	}
	sort.Slice(data.Aliases, func(a, b int) bool {
		if data.Aliases[a].NormalizedAlias == data.Aliases[b].NormalizedAlias {
			return data.Aliases[a].Slug < data.Aliases[b].Slug
		}
		return data.Aliases[a].NormalizedAlias < data.Aliases[b].NormalizedAlias
	})
	report.PageCount = len(pages)
	return data, report, nil
}

func ExtractExplicitLinks(body string) []ExplicitLink {
	matches := wikiLinkRE.FindAllStringSubmatchIndex(body, -1)
	links := make([]ExplicitLink, 0, len(matches))
	for _, match := range matches {
		target := body[match[2]:match[3]]
		display := ""
		if match[4] >= 0 {
			display = body[match[4]:match[5]]
		}
		target = strings.TrimSpace(strings.SplitN(target, "#", 2)[0])
		if target == "" {
			continue
		}
		links = append(links, ExplicitLink{
			Target:  target,
			Display: strings.TrimSpace(display),
			Context: contextAround(body, match[0], match[1]),
		})
	}
	return links
}

func StripCode(body string) string {
	body = fencedCodeRE.ReplaceAllStringFunc(body, func(s string) string {
		return strings.Repeat(" ", len(s))
	})
	body = inlineCodeRE.ReplaceAllStringFunc(body, func(s string) string {
		return strings.Repeat(" ", len(s))
	})
	return body
}

func SplitChunks(page memfs.Page) []sqlitestore.ChunkRecord {
	text := strings.TrimSpace(page.Body)
	if text == "" {
		text = page.Title
	}
	paragraphs := paragraphRE.Split(text, -1)
	var chunks []sqlitestore.ChunkRecord
	var b strings.Builder
	flush := func() {
		chunk := strings.TrimSpace(b.String())
		if chunk == "" {
			return
		}
		sum := sha256.Sum256([]byte(chunk))
		chunks = append(chunks, sqlitestore.ChunkRecord{
			Slug:       page.Slug,
			Index:      len(chunks),
			Title:      page.Title,
			Body:       chunk,
			BodyHash:   hex.EncodeToString(sum[:]),
			ModifiedAt: page.ModifiedAt,
		})
		b.Reset()
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if b.Len() > 0 && b.Len()+len(paragraph)+2 > 1400 {
			flush()
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(paragraph)
	}
	flush()
	if len(chunks) == 0 {
		sum := sha256.Sum256([]byte(page.Title))
		chunks = append(chunks, sqlitestore.ChunkRecord{
			Slug:       page.Slug,
			Index:      0,
			Title:      page.Title,
			Body:       page.Title,
			BodyHash:   hex.EncodeToString(sum[:]),
			ModifiedAt: page.ModifiedAt,
		})
	}
	return chunks
}

func NormalizeAlias(alias string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(alias))), " ")
}

type resolver struct {
	slugs map[string]bool
	names map[string][]string
}

func newResolver(pages []memfs.Page) resolver {
	r := resolver{slugs: map[string]bool{}, names: map[string][]string{}}
	for _, page := range pages {
		r.slugs[page.Slug] = true
		for _, alias := range aliasesForPage(page) {
			normalized := NormalizeAlias(alias)
			if normalized == "" {
				continue
			}
			r.names[normalized] = appendUnique(r.names[normalized], page.Slug)
		}
	}
	return r
}

func (r resolver) Resolve(target string) (string, string) {
	target = strings.TrimSpace(target)
	clean := memfs.CleanSlug(target)
	if clean == "" {
		return "", "empty"
	}
	if r.slugs[clean] {
		return clean, ""
	}
	normalized := NormalizeAlias(target)
	matches := r.names[normalized]
	switch len(matches) {
	case 0:
		return "", "dangling"
	case 1:
		return matches[0], ""
	default:
		return "", "ambiguous"
	}
}

type gazetteerEntry struct {
	Slug  string
	Alias string
	RE    *regexp.Regexp
}

func buildGazetteer(pages []memfs.Page) []gazetteerEntry {
	var entries []gazetteerEntry
	seen := map[string]bool{}
	for _, page := range pages {
		for _, alias := range aliasesForPage(page) {
			alias = strings.TrimSpace(alias)
			if !mentionAliasAllowed(alias) {
				continue
			}
			key := page.Slug + "\x00" + strings.ToLower(alias)
			if seen[key] {
				continue
			}
			seen[key] = true
			entries = append(entries, gazetteerEntry{
				Slug:  page.Slug,
				Alias: alias,
				RE:    mentionRegexp(alias),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].Alias) == len(entries[j].Alias) {
			return entries[i].Alias < entries[j].Alias
		}
		return len(entries[i].Alias) > len(entries[j].Alias)
	})
	return entries
}

func detectMentions(page memfs.Page, body string, entries []gazetteerEntry) []sqlitestore.LinkRecord {
	var out []sqlitestore.LinkRecord
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Slug == page.Slug {
			continue
		}
		match := entry.RE.FindStringIndex(body)
		if match == nil {
			continue
		}
		key := entry.Slug + "\x00" + strings.ToLower(entry.Alias)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sqlitestore.LinkRecord{
			FromSlug:   page.Slug,
			ToSlug:     entry.Slug,
			LinkType:   "mention",
			LinkSource: "mention",
			Display:    entry.Alias,
			Context:    contextAround(body, match[0], match[1]),
		})
	}
	return out
}

func aliasesForPage(page memfs.Page) []string {
	values := append([]string{page.Title, slugTail(page.Slug)}, page.Aliases...)
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := NormalizeAlias(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func slugTail(slug string) string {
	parts := strings.Split(slug, "/")
	return strings.ReplaceAll(parts[len(parts)-1], "-", " ")
}

func mentionAliasAllowed(alias string) bool {
	normalized := NormalizeAlias(alias)
	if len(normalized) < 4 {
		return false
	}
	if strings.Count(normalized, " ") == 0 && len(normalized) < 4 {
		return false
	}
	return true
}

func mentionRegexp(alias string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(alias)
	return regexp.MustCompile(`(?i)(^|[^[:alnum:]_])` + escaped + `([^[:alnum:]_]|$)`)
}

func contextAround(body string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(body) {
		end = len(body)
	}
	left := start - 90
	if left < 0 {
		left = 0
	}
	right := end + 90
	if right > len(body) {
		right = len(body)
	}
	context := strings.TrimSpace(body[left:right])
	context = strings.Join(strings.Fields(context), " ")
	if len(context) > 220 {
		context = context[:220]
	}
	return context
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

var (
	wikiLinkRE   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	fencedCodeRE = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRE = regexp.MustCompile("`[^`]*`")
	paragraphRE  = regexp.MustCompile(`\n{2,}`)
)
