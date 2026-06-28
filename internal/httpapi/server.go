package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/mcpapi"
	"github.com/gluonfield/jazmem/pkg/jazmem"
)

type Server struct {
	Memory *jazmem.Memory
}

func New(memory *jazmem.Memory) http.Handler {
	s := &Server{Memory: memory}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /doctor", s.doctor)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /page/", s.getPage)
	mux.HandleFunc("GET /file/", s.getFile)
	mux.HandleFunc("GET /tasks", s.tasks)
	mux.HandleFunc("POST /reindex", s.reindex)
	mux.HandleFunc("POST /dream", s.dream)
	mux.HandleFunc("POST /link-hygiene", s.linkHygiene)
	mux.Handle("/mcp", mcpapi.NewHTTPHandler(memory))
	return recoverJSON(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"root":   s.Memory.Root(),
		"db":     s.Memory.DBPath(),
	})
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	report, err := s.Memory.Doctor(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if query == "" {
		writeError(w, errors.New("query parameter q is required"))
		return
	}
	deep := isTruthy(r.URL.Query().Get("deep"))
	if isTruthy(r.URL.Query().Get("agentic")) {
		results, err := s.Memory.AgenticSearch(r.Context(), query, jazmem.AgenticOptions{Deep: deep})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, results)
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, err)
			return
		}
		limit = parsed
	}
	results, err := s.Memory.Retrieve(r.Context(), query, jazmem.SearchOptions{Limit: limit, Deep: deep})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/page/"), "/")
	if slug == "" {
		writeError(w, errors.New("page slug is required"))
		return
	}
	page, err := s.Memory.GetPage(r.Context(), slug)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.Memory.ListTasks(r.Context(), jazmem.TaskFilter{Status: r.URL.Query().Get("status")})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/file/"), "/")
	if slug == "" {
		writeError(w, errors.New("page slug is required"))
		return
	}
	page, err := s.Memory.GetPage(r.Context(), slug)
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Query().Get("raw") == "1" || r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(page.Raw))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"slug": page.Slug,
		"path": page.Path,
	})
}

func (s *Server) reindex(w http.ResponseWriter, r *http.Request) {
	report, err := s.Memory.Reindex(r.Context(), jazmem.ReindexOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) dream(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Date string `json:"date"`
	}
	if err := readOptionalJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	var date time.Time
	if strings.TrimSpace(input.Date) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.Date))
		if err != nil {
			writeError(w, err)
			return
		}
		date = parsed
	}
	report, err := s.Memory.Dream(r.Context(), jazmem.DreamOptions{Date: date})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) linkHygiene(w http.ResponseWriter, r *http.Request) {
	report, err := s.Memory.LinkHygiene(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func readJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func readOptionalJSON(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return readJSON(r, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, http.ErrNoLocation) {
		status = http.StatusInternalServerError
	}
	var notFound *jazmem.NotFoundError
	if errors.As(err, &notFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":       "not found: " + notFound.Slug,
			"suggestions": notFound.Suggestions,
		})
		return
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func recoverJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
