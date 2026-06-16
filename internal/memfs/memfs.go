package memfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type FileSystem struct {
	Root string
}

type LayoutReport struct {
	Directories []string
	Created     []string
	Existing    []string
}

type Page struct {
	Slug        string
	RelPath     string
	AbsPath     string
	Type        string
	Title       string
	Aliases     []string
	Frontmatter map[string]any
	Body        string
	Raw         string
	BodyHash    string
	ModifiedAt  time.Time
}

func New(root string) *FileSystem {
	return &FileSystem{Root: root}
}

func (fs *FileSystem) EnsureLayout() error {
	_, err := fs.EnsureLayoutReport()
	return err
}

func (fs *FileSystem) EnsureLayoutReport() (LayoutReport, error) {
	report := LayoutReport{
		Directories: []string{},
		Created:     []string{},
		Existing:    []string{},
	}
	for _, dir := range LayoutDirs() {
		report.Directories = append(report.Directories, dir)
		path := filepath.Join(fs.Root, dir)
		if _, err := os.Stat(path); err == nil {
			report.Existing = append(report.Existing, dir)
		} else if os.IsNotExist(err) {
			report.Created = append(report.Created, dir)
		} else {
			return LayoutReport{}, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return LayoutReport{}, err
		}
	}
	return report, nil
}

const (
	LongTermFile      = "LONG_TERM.md"
	ShortTermFile     = "SHORT_TERM.md"
	LongTermMaxChars  = 2500
	ShortTermMaxChars = 1500
	longTermHeading   = "# Long Term Memory"
	shortTermHeading  = "# Short Term Memory"
)

var horizonSkeletons = map[string]string{
	LongTermFile: longTermHeading + `

Identity, goals, standing preferences, key relationships. Maintained periodically by dream; agents treat this file as read-only.
`,
	ShortTermFile: shortTermHeading + `

Current focus, active projects, open loops. Agents update entries in place as the present changes; dream prunes stale ones.
`,
}

// HorizonFiles are root-level injection surfaces, not pages: they are excluded
// from page listing/indexing and are read or written whole.
func HorizonFiles() []string {
	return []string{LongTermFile, ShortTermFile}
}

func HorizonMaxChars(name string) (int, bool) {
	switch name {
	case LongTermFile:
		return LongTermMaxChars, true
	case ShortTermFile:
		return ShortTermMaxChars, true
	default:
		return 0, false
	}
}

func HorizonHeading(name string) (string, bool) {
	switch name {
	case LongTermFile:
		return longTermHeading, true
	case ShortTermFile:
		return shortTermHeading, true
	default:
		return "", false
	}
}

func ValidateHorizonContent(name, content string) error {
	maxChars, ok := HorizonMaxChars(name)
	if !ok {
		return fmt.Errorf("unknown horizon file %q", name)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("%s content is empty", name)
	}
	if len(content) > maxChars {
		return fmt.Errorf("%s exceeds %d chars", name, maxChars)
	}
	heading, _ := HorizonHeading(name)
	firstLine, _, _ := strings.Cut(content, "\n")
	if strings.TrimSpace(firstLine) != heading {
		return fmt.Errorf("%s must start with %q", name, heading)
	}
	return nil
}

func isHorizonPath(root, path string) bool {
	for _, name := range HorizonFiles() {
		if path == filepath.Join(root, name) {
			return true
		}
	}
	return false
}

func (fs *FileSystem) EnsureHorizonFiles() ([]string, error) {
	// Callers may run this before EnsureLayout on a fresh machine, where even
	// the root doesn't exist yet.
	if err := os.MkdirAll(fs.Root, 0o755); err != nil {
		return nil, err
	}
	var created []string
	for _, name := range HorizonFiles() {
		path := filepath.Join(fs.Root, name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(horizonSkeletons[name]), 0o644); err != nil {
			return nil, err
		}
		created = append(created, name)
	}
	return created, nil
}

func (fs *FileSystem) ReadRootFile(name string) (string, error) {
	path, err := fs.rootFilePath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (fs *FileSystem) WriteRootFile(name, content string) error {
	path, err := fs.rootFilePath(name)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return AtomicWrite(path, []byte(content))
}

func (fs *FileSystem) rootFilePath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid root file name %q", name)
	}
	return filepath.Join(fs.Root, name), nil
}

func LayoutDirs() []string {
	return []string{
		"daily",
		"inbox",
		"sources",
		"sources/email",
		"sources/chat",
		"sources/agent",
		"people",
		"companies",
		"projects",
		"concepts",
		"notes",
		"dreams",
		"dreams/runs",
		"dreams/review",
	}
}

func (fs *FileSystem) ListPages() ([]Page, error) {
	var pages []Page
	if err := filepath.WalkDir(fs.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != fs.Root {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if isHorizonPath(fs.Root, path) {
			return nil
		}
		page, err := fs.ReadPath(path)
		if err != nil {
			return err
		}
		pages = append(pages, page)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Slug < pages[j].Slug })
	return pages, nil
}

func (fs *FileSystem) ReadPage(slug string) (Page, error) {
	path, err := fs.PathForSlug(slug)
	if err != nil {
		return Page{}, err
	}
	return fs.ReadPath(path)
}

func (fs *FileSystem) ReadPath(path string) (Page, error) {
	rel, err := filepath.Rel(fs.Root, path)
	if err != nil {
		return Page{}, err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return Page{}, fmt.Errorf("path %s escapes memory root", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Page{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Page{}, err
	}
	return Parse(rel, path, string(data), info.ModTime())
}

func (fs *FileSystem) WritePage(slug, content string) error {
	path, err := fs.PathForSlug(slug)
	if err != nil {
		return err
	}
	return AtomicWrite(path, []byte(content))
}

func (fs *FileSystem) PathForSlug(slug string) (string, error) {
	slug = CleanSlug(slug)
	if slug == "" {
		return "", errors.New("slug is empty")
	}
	path := filepath.Join(fs.Root, filepath.FromSlash(slug)+".md")
	rel, err := filepath.Rel(fs.Root, path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("slug %q escapes memory root", slug)
	}
	return path, nil
}

func Parse(relPath, absPath, raw string, modifiedAt time.Time) (Page, error) {
	relPath = filepath.ToSlash(relPath)
	slug := strings.TrimSuffix(relPath, ".md")
	body := raw
	fm := map[string]any{}
	if parsed, rest, ok := parseFrontmatter(raw); ok {
		fm = parsed
		body = rest
	}
	title := firstString(fm["title"])
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = titleFromSlug(slug)
	}
	typ := firstString(fm["type"])
	if typ == "" {
		typ = pageType(slug)
	}
	aliases := uniqueStrings(stringSlice(fm["aliases"]))
	sum := sha256.Sum256([]byte(body))
	return Page{
		Slug:        slug,
		RelPath:     relPath,
		AbsPath:     absPath,
		Type:        typ,
		Title:       title,
		Aliases:     aliases,
		Frontmatter: fm,
		Body:        body,
		Raw:         raw,
		BodyHash:    hex.EncodeToString(sum[:]),
		ModifiedAt:  modifiedAt,
	}, nil
}

func AtomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func CleanSlug(slug string) string {
	slug = filepath.ToSlash(strings.TrimSpace(slug))
	slug = strings.TrimSuffix(slug, ".md")
	for strings.Contains(slug, "//") {
		slug = strings.ReplaceAll(slug, "//", "/")
	}
	slug = strings.Trim(slug, "/")
	return slug
}

func Slugify(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastDash := false
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func FrontmatterString(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("---\n")
	for _, key := range keys {
		if values[key] == "" {
			continue
		}
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(escapeFrontmatterScalar(values[key]))
		b.WriteByte('\n')
	}
	b.WriteString("---\n\n")
	return b.String()
}

func parseFrontmatter(raw string) (map[string]any, string, bool) {
	if !strings.HasPrefix(raw, "---\n") && !strings.HasPrefix(raw, "---\r\n") {
		return nil, raw, false
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.SplitAfter(normalized, "\n")
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		nextOffset := offset + len(lines[i])
		if line != "---" {
			offset = nextOffset
			continue
		}
		block := normalized[len(lines[0]):offset]
		rest := normalized[nextOffset:]
		out := map[string]any{}
		if strings.TrimSpace(block) != "" {
			if err := yaml.Unmarshal([]byte(block), &out); err != nil {
				return nil, raw, false
			}
		}
		return out, rest, true
	}
	return nil, raw, false
}

func stringSlice(v any) []string {
	switch value := v.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			s := firstString(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	default:
		return nil
	}
}

func firstString(v any) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
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

func firstHeading(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		if after, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func titleFromSlug(slug string) string {
	parts := strings.Split(slug, "/")
	last := parts[len(parts)-1]
	words := strings.Fields(strings.ReplaceAll(last, "-", " "))
	if len(words) == 0 {
		return slug
	}
	for i, word := range words {
		if len(word) > 0 && word[0] >= 'a' && word[0] <= 'z' {
			words[i] = string(word[0]-32) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func pageType(slug string) string {
	if i := strings.Index(slug, "/"); i > 0 {
		return slug[:i]
	}
	return "notes"
}

var frontmatterUnsafe = regexp.MustCompile(`[:\[\],{}#&*!|>'"%@` + "`" + `]`)

func escapeFrontmatterScalar(value string) string {
	if value == "" {
		return `""`
	}
	if frontmatterUnsafe.MatchString(value) || strings.Contains(value, "\n") || strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		return `"` + value + `"`
	}
	return value
}
