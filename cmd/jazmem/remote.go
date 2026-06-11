package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
)

type httpBackend struct {
	base   string
	client *http.Client
}

func newHTTPBackend(base string) *httpBackend {
	// Dream and link-hygiene call an LLM; give long operations room.
	return &httpBackend{base: base, client: &http.Client{Timeout: 3 * time.Minute}}
}

func (b *httpBackend) Retrieve(ctx context.Context, query string, opts jazmem.SearchOptions) (jazmem.SearchResponse, error) {
	params := url.Values{"q": {query}}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Deep {
		params.Set("deep", "1")
	}
	var out jazmem.SearchResponse
	err := b.do(ctx, http.MethodGet, "/search?"+params.Encode(), &out)
	return out, err
}

func (b *httpBackend) AgenticSearch(ctx context.Context, query string, opts jazmem.AgenticOptions) (jazmem.AgenticResponse, error) {
	params := url.Values{"q": {query}, "agentic": {"1"}}
	if opts.Deep {
		params.Set("deep", "1")
	}
	var out jazmem.AgenticResponse
	err := b.do(ctx, http.MethodGet, "/search?"+params.Encode(), &out)
	return out, err
}

func (b *httpBackend) GetPage(ctx context.Context, slug string) (jazmem.Page, error) {
	var out jazmem.Page
	err := b.do(ctx, http.MethodGet, "/page/"+url.PathEscape(slug), &out)
	return out, err
}

func (b *httpBackend) Reindex(ctx context.Context) (jazmem.Report, error) {
	var out jazmem.Report
	err := b.do(ctx, http.MethodPost, "/reindex", &out)
	return out, err
}

func (b *httpBackend) Doctor(ctx context.Context) (jazmem.DoctorReport, error) {
	var out jazmem.DoctorReport
	err := b.do(ctx, http.MethodGet, "/doctor", &out)
	return out, err
}

func (b *httpBackend) Dream(ctx context.Context) (jazmem.DreamReport, error) {
	var out jazmem.DreamReport
	err := b.do(ctx, http.MethodPost, "/dream", &out)
	return out, err
}

func (b *httpBackend) LinkHygiene(ctx context.Context) (jazmem.LinkHygieneReport, error) {
	var out jazmem.LinkHygieneReport
	err := b.do(ctx, http.MethodPost, "/link-hygiene", &out)
	return out, err
}

func (b *httpBackend) Close() error { return nil }

func (b *httpBackend) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return remoteError(resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

// remoteError reconstructs NotFoundError so remote get/file keeps the CLI's
// suggestion output; other failures surface the server's error text.
func remoteError(status int, body []byte) error {
	var payload struct {
		Error       string                  `json:"error"`
		Suggestions []jazmem.SlugSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		if status == http.StatusNotFound {
			if slug, ok := strings.CutPrefix(payload.Error, "not found: "); ok {
				return &jazmem.NotFoundError{Slug: slug, Suggestions: payload.Suggestions}
			}
		}
		return fmt.Errorf("server: %s", payload.Error)
	}
	return fmt.Errorf("server returned status %d", status)
}
