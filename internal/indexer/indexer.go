package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

// frontmatterLinkFields are the frontmatter keys whose values are page
// references, so a page's structured links (a task's project) ride the same
// graph as body wikilinks without being duplicated into prose.
var frontmatterLinkFields = []string{"project"}

const extractorHash = "jazmem-indexer-v1"

type Indexer struct {
	FS    *memfs.FileSystem
	Store *sqlitestore.Store
}

type Report struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	ExplicitLinks   int `json:"explicit_links"`
	TypedLinks      int `json:"typed_links"`
	MentionLinks    int `json:"mention_links"`
	UnresolvedLinks int `json:"unresolved_links"`
}

type ExplicitLink struct {
	Target          string
	Display         string
	Context         string
	InRelationships bool
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
			if link.InRelationships {
				data.Links = append(data.Links, inferTypedLinks(page.Slug, target, link)...)
			}
		}

		for _, target := range frontmatterLinkTargets(page.Frontmatter) {
			resolved, reason := resolver.Resolve(target)
			if resolved == "" {
				data.Unresolved = append(data.Unresolved, sqlitestore.UnresolvedLinkRecord{
					FromSlug: page.Slug,
					Target:   memfs.CleanSlug(target),
					Reason:   reason,
					Context:  "frontmatter",
				})
				report.UnresolvedLinks++
				continue
			}
			if resolved == page.Slug {
				continue
			}
			data.Links = append(data.Links, sqlitestore.LinkRecord{
				FromSlug:   page.Slug,
				ToSlug:     resolved,
				LinkType:   "reference",
				LinkSource: "explicit",
				Display:    slugTail(resolved),
				Context:    "frontmatter",
			})
		}

		for _, mention := range detectMentions(page, clean, gazetteer) {
			if !slugSet[mention.ToSlug] || mention.ToSlug == page.Slug {
				continue
			}
			data.Links = append(data.Links, mention)
		}

		chunks := SplitChunks(page)
		data.Chunks = append(data.Chunks, chunks...)
		report.ChunkCount += len(chunks)
	}
	data.Links = dedupeLinks(data.Links)
	for _, link := range data.Links {
		switch link.LinkSource {
		case "explicit":
			report.ExplicitLinks++
		case "relationship":
			report.TypedLinks++
		case "mention":
			report.MentionLinks++
		}
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

// dedupeLinks keeps the first row per (from, to, type, source): reciprocal
// relationship bullets and multi-alias mentions otherwise insert duplicate
// edges because the links primary key includes display and context.
func dedupeLinks(links []sqlitestore.LinkRecord) []sqlitestore.LinkRecord {
	seen := map[string]bool{}
	out := make([]sqlitestore.LinkRecord, 0, len(links))
	for _, link := range links {
		key := link.FromSlug + "\x00" + link.ToSlug + "\x00" + link.LinkType + "\x00" + link.LinkSource
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, link)
	}
	return out
}

func ExtractExplicitLinks(body string) []ExplicitLink {
	var links []ExplicitLink
	relationshipLevel := 0
	for line := range strings.SplitSeq(body, "\n") {
		if level, heading := markdownHeading(line); level > 0 {
			switch {
			case heading == "relationships" || heading == "relations":
				relationshipLevel = level
			case relationshipLevel > 0 && level <= relationshipLevel:
				relationshipLevel = 0
			}
		}
		matches := wikiLinkRE.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			target := line[match[2]:match[3]]
			display := ""
			if match[4] >= 0 {
				display = line[match[4]:match[5]]
			}
			target = strings.TrimSpace(strings.SplitN(target, "#", 2)[0])
			if target == "" {
				continue
			}
			context := strings.TrimSpace(line)
			context = strings.Join(strings.Fields(context), " ")
			if len(context) > 240 {
				context = context[:240]
			}
			links = append(links, ExplicitLink{
				Target:          target,
				Display:         strings.TrimSpace(display),
				Context:         context,
				InRelationships: relationshipLevel > 0 && !sourceMarkerBefore(line, match[0]),
			})
		}
	}
	return links
}

// frontmatterLinkTargets pulls page references out of a page's structured
// frontmatter fields. It accepts both [[wikilinks]] and bare slugs (anything
// with a "/" lane prefix), skipping free text so a prose value does not
// register as a dangling link.
func frontmatterLinkTargets(fm map[string]any) []string {
	var targets []string
	for _, key := range frontmatterLinkFields {
		value, ok := fm[key]
		if !ok || value == nil {
			continue
		}
		raw := strings.TrimSpace(fmt.Sprint(value))
		if raw == "" {
			continue
		}
		if matches := wikiLinkRE.FindAllStringSubmatch(raw, -1); len(matches) > 0 {
			for _, match := range matches {
				if target := strings.TrimSpace(strings.SplitN(match[1], "#", 2)[0]); target != "" {
					targets = append(targets, target)
				}
			}
			continue
		}
		if strings.Contains(raw, "/") {
			targets = append(targets, raw)
		}
	}
	return targets
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

type relationSpec struct {
	Type      string
	Symmetric bool
	Orient    string
}

func inferTypedLinks(fromSlug, toSlug string, link ExplicitLink) []sqlitestore.LinkRecord {
	spec := inferRelation(link.Context)
	if spec.Type == "" {
		return nil
	}
	if !isTypedRelationshipNode(fromSlug) || !isTypedRelationshipNode(toSlug) {
		return nil
	}
	if spec.Type == "friend" && (!isPersonSlug(fromSlug) || !isPersonSlug(toSlug)) {
		return nil
	}
	context := strings.TrimSpace(link.Context)
	display := strings.TrimSpace(link.Display)
	if display == "" {
		display = slugTail(toSlug)
	}
	makeRecord := func(from, to string) sqlitestore.LinkRecord {
		return sqlitestore.LinkRecord{
			FromSlug:   from,
			ToSlug:     to,
			LinkType:   spec.Type,
			LinkSource: "relationship",
			Display:    display,
			Context:    context,
		}
	}
	if spec.Symmetric {
		return []sqlitestore.LinkRecord{
			makeRecord(fromSlug, toSlug),
			makeRecord(toSlug, fromSlug),
		}
	}
	from, to := orientRelationship(fromSlug, toSlug, spec.Orient)
	if from == "" || to == "" || from == to {
		return nil
	}
	return []sqlitestore.LinkRecord{makeRecord(from, to)}
}

func inferRelation(context string) relationSpec {
	text := strings.ToLower(context)
	switch {
	case containsAny(text, "friend", "friends"):
		return relationSpec{Type: "friend", Symmetric: true}
	case containsAny(text, "works with", "worked with", "collaborates", "collaborator", "collaboration"):
		return relationSpec{Type: "works_with", Symmetric: true}
	case containsAny(text, "works at", "works for", "worked at", "worked for", "employed by", "employee at", "director of", "director at", "head of"):
		return relationSpec{Type: "works_at", Orient: "person_to_org"}
	case containsAny(text, "founded", "founder", "co-founder", "cofounder", "started"):
		return relationSpec{Type: "founder_of", Orient: "actor_to_org"}
	case containsAny(text, "invested in", "invests in", "investor", "investment"):
		return relationSpec{Type: "invested_in", Orient: "actor_to_org"}
	case containsAny(text, "advises", "advised", "advisor", "advisory", "board member", "on the board"):
		return relationSpec{Type: "advises", Orient: "actor_to_org"}
	default:
		return relationSpec{}
	}
}

// orientRelationship returns empty slugs when neither node fits the relation
// shape (e.g. a person-to-person "founder" mention), so no edge is emitted
// rather than a misdirected one.
func orientRelationship(fromSlug, toSlug, orient string) (string, string) {
	switch orient {
	case "person_to_org":
		if isPersonSlug(fromSlug) && isOrgSlug(toSlug) {
			return fromSlug, toSlug
		}
		if isPersonSlug(toSlug) && isOrgSlug(fromSlug) {
			return toSlug, fromSlug
		}
	case "actor_to_org":
		if isActorSlug(fromSlug) && isOrgSlug(toSlug) {
			return fromSlug, toSlug
		}
		if isActorSlug(toSlug) && isOrgSlug(fromSlug) {
			return toSlug, fromSlug
		}
	}
	return "", ""
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isPersonSlug(slug string) bool {
	return strings.HasPrefix(slug, "people/")
}

func isOrgSlug(slug string) bool {
	return strings.HasPrefix(slug, "companies/") || strings.HasPrefix(slug, "projects/")
}

func isActorSlug(slug string) bool {
	return isPersonSlug(slug) || strings.HasPrefix(slug, "companies/")
}

func isTypedRelationshipNode(slug string) bool {
	return isPersonSlug(slug) || isOrgSlug(slug) || strings.HasPrefix(slug, "concepts/")
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
	start = max(start, 0)
	end = min(end, len(body))
	left := max(start-90, 0)
	right := min(end+90, len(body))
	context := strings.TrimSpace(body[left:right])
	context = strings.Join(strings.Fields(context), " ")
	if len(context) > 220 {
		context = context[:220]
	}
	return context
}

func markdownHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, ""
	}
	level := 0
	for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, ""
	}
	heading := strings.TrimSpace(trimmed[level:])
	heading = strings.TrimSpace(headingHashSuffixRE.ReplaceAllString(heading, ""))
	heading = strings.ToLower(heading)
	heading = strings.Join(strings.Fields(heading), " ")
	return level, heading
}

func sourceMarkerBefore(line string, offset int) bool {
	if offset <= 0 {
		return false
	}
	prefix := strings.ToLower(line[:offset])
	return strings.Contains(prefix, "[source:") || strings.Contains(prefix, "source:")
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

var (
	wikiLinkRE   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	fencedCodeRE = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRE = regexp.MustCompile("`[^`]*`")
	paragraphRE  = regexp.MustCompile(`\n{2,}`)

	headingHashSuffixRE = regexp.MustCompile(`\s+#+\s*$`)
)
