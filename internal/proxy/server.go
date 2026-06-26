package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	store   *SettingsStore
	logger  *Logger
	backend *Backend
}

func NewServer(store *SettingsStore, logger *Logger) *Server {
	return &Server{
		store:   store,
		logger:  logger,
		backend: NewBackend(store, logger),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := r.URL.Path
	switch {
	case path == "/":
		http.Redirect(w, r, "/settings", http.StatusFound)
	case path == "/settings":
		s.handleSettingsPage(w, r)
	case path == "/api/settings":
		s.handleSettingsAPI(w, r)
	case path == "/api/backend/status":
		s.handleBackendStatus(w, r)
	case path == "/version":
		s.handleVersion(w, r)
	case path == "/lastupdatedb":
		s.handleLastUpdateDB(w, r)
	case path == "/api":
		s.handleTorznabRequest(w, r, "all")
	case path == "/api/v2.0/indexers":
		s.handleJackettIndexersList(w, r)
	case path == "/api/v1/indexer":
		s.handleProwlarrIndexersStub(w, r)
	case strings.HasPrefix(path, "/api/v2.0/indexers/"):
		s.handleDynamicIndexerPath(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDynamicIndexerPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v2.0/indexers/")
	if strings.HasSuffix(rest, "/results/torznab/api") {
		indexerID := strings.SplitN(rest, "/", 2)[0]
		if indexerID == "" {
			indexerID = "all"
		}
		s.handleTorznabRequest(w, r, indexerID)
		return
	}
	if strings.HasSuffix(rest, "/results") && !strings.Contains(strings.TrimSuffix(rest, "/results"), "/") {
		status := strings.TrimSuffix(rest, "/results")
		s.handleJackettResults(w, r, status)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleTorznabRequest(w http.ResponseWriter, r *http.Request, indexerID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings := s.store.Get()
	q := r.URL.Query()
	t := q.Get("t")
	apikey := q.Get("apikey")
	catParam := q.Get("cat")

	s.logger.Infof("[TORZNAB] indexer=%s t=%s apikey=%v path=%s", indexerID, t, apikey != "", r.URL.Path)
	s.logger.Debugf("[TORZNAB] args: %s", q.Encode())

	if t == "caps" {
		writeXML(w, getCapsXML(r))
		return
	}
	if t == "indexers" {
		configured := strings.ToLower(q.Get("configured"))
		if configured == "" || configured == "true" {
			writeXML(w, getIndexersXML())
			return
		}
		writeXML(w, `<?xml version="1.0" encoding="UTF-8"?><indexers></indexers>`)
		return
	}

	assignedCat := ""
	switch t {
	case "tvsearch":
		assignedCat = "5000"
	case "moviesearch":
		assignedCat = "2000"
	default:
		if catParam != "" {
			assignedCat = strings.TrimSpace(strings.Split(catParam, ",")[0])
		}
	}

	tnParams := torznabSearchParamsFromValues(q, s.logger)
	query := tnParams.Query
	title := q.Get("title")
	titleOriginal := q.Get("title_original")
	year := yearFromValues(q)
	if year <= 0 {
		year = tnParams.Year
	}
	if tnParams.TVDBIDOnly {
		s.logger.Warningf("[TORZNAB] tvdbid-only search - returning empty")
		writeXML(w, wrapInXML(""))
		return
	}
	if query == "" && title == "" && titleOriginal == "" {
		s.logger.Warningf("[TORZNAB] search without query/title")
		writeXML(w, wrapInXML(""))
		return
	}

	isSerial := isSerialFromSearch(t, catParam)
	var explicitIsSerial *int
	if raw := q.Get("is_serial"); raw != "" {
		if value, ok := optionalInt(raw); ok {
			isSerial = value
			explicitIsSerial = &isSerial
		}
	}
	genres := q.Get("genres")
	categories := categoriesFromValues(q)
	cardMode := isCardMetadataSearch(title, titleOriginal, explicitIsSerial, categories, genres)

	s.logger.Infof("[TORZNAB] search t=%s q=%q title=%q orig=%q year=%d season=%d ep=%d is_serial=%d cat=%s",
		t, query, title, titleOriginal, year, tnParams.Season, tnParams.Episode, isSerial, firstNonEmpty(catParam, "(none)"))

	torrents := s.backend.SearchCombined(r.Context(), SearchOptions{
		APIKey:        apikey,
		Query:         query,
		Title:         title,
		TitleOriginal: titleOriginal,
		Year:          year,
		IsSerial:      isSerial,
		Genres:        genres,
		Categories:    categories,
		Season:        tnParams.Season,
		CardMode:      &cardMode,
		Settings:      settings,
	})
	if isSerial < 0 && catParam != "" && !cardMode && !settings.SkipCatFilter {
		torrents = filterResultsByCategory(torrents, catParam, s.logger)
	} else if settings.SkipCatFilter && catParam != "" && !cardMode {
		s.logger.Infof("[TORZNAB] category filter skipped (JACRED_SKIP_CAT_FILTER)")
	}
	if year > 0 && !cardMode {
		torrents = filterResultsByYear(torrents, year, s.logger)
	}
	if tnParams.HasSeason {
		torrents = filterResultsBySeasonEpisode(torrents, tnParams.Season, tnParams.Episode, tnParams.HasEpisode, s.logger)
	}
	torrents = paginateResults(torrents, tnParams.Limit, tnParams.Offset, tnParams.HasLimit, tnParams.HasOffset, s.logger)
	s.logger.Infof("[TORZNAB] %d results after merge (+filters)", len(torrents))

	var builder strings.Builder
	for _, torrent := range torrents {
		builder.WriteString(torrentToXMLItem(torrent, assignedCat, catParam, settings))
	}
	writeXML(w, wrapInXML(builder.String()))
}

func (s *Server) handleJackettIndexersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings := s.store.Get()
	writeJSON(w, http.StatusOK, []map[string]any{
		{
			"id":          "all",
			"name":        "JacRed (all trackers)",
			"description": "JacRed torrent aggregator via JacPro",
			"type":        "public",
			"configured":  true,
			"site_link":   settings.BaseURL,
			"language":    "ru-RU",
		},
	})
}

func (s *Server) handleProwlarrIndexersStub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{
		{
			"id":             1,
			"name":           "JacRed (all)",
			"enable":         true,
			"protocol":       "torrent",
			"supportsSearch": true,
		},
	})
}

func (s *Server) handleJackettResults(w http.ResponseWriter, r *http.Request, status string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings := s.store.Get()
	q := r.URL.Query()
	query := firstNonEmpty(q.Get("Query"), q.Get("q"), q.Get("query"))
	title := q.Get("title")
	titleOriginal := q.Get("title_original")
	year := yearFromValues(q)
	apikey := q.Get("apikey")
	tracker := firstNonEmpty(q.Get("Tracker"), q.Get("tracker"))

	if query == "" && title == "" && titleOriginal == "" {
		s.logger.Warningf("[JACKETT] called without query/title")
		writeJSON(w, http.StatusOK, map[string]any{"Results": []any{}})
		return
	}

	isSerial := -1
	var explicitIsSerial *int
	if raw := q.Get("is_serial"); raw != "" {
		if value, ok := optionalInt(raw); ok {
			isSerial = value
			explicitIsSerial = &isSerial
		}
	}
	genres := q.Get("genres")
	categories := categoriesFromValues(q)
	cardMode := isCardMetadataSearch(title, titleOriginal, explicitIsSerial, categories, genres)
	mode := "fuzzy"
	if cardMode {
		mode = "card"
	}
	s.logger.Infof("[JACKETT] indexer=%s mode=%s q=%q title=%q orig=%q year=%d is_serial=%d categories=%v",
		status, mode, query, title, titleOriginal, year, isSerial, categories)

	torrents := s.backend.SearchCombined(r.Context(), SearchOptions{
		APIKey:        apikey,
		Query:         query,
		Title:         title,
		TitleOriginal: titleOriginal,
		Year:          year,
		Tracker:       tracker,
		IsSerial:      isSerial,
		Genres:        genres,
		Categories:    categories,
		CardMode:      &cardMode,
		Settings:      settings,
	})
	catParam := firstNonEmpty(q.Get("cat"), q.Get("Category"))
	if catParam != "" && !cardMode {
		torrents = filterResultsByCategory(torrents, catParam, s.logger)
	}
	results := make([]map[string]any, 0, len(torrents))
	for _, torrent := range torrents {
		results = append(results, resultToJackettJSON(torrent))
	}
	s.logger.Infof("[JACKETT] returning %d results", len(results))
	writeJSON(w, http.StatusOK, map[string]any{"Results": results})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings := s.store.Get()
	if data, ok := s.forwardBackendJSON(r.Context(), "/version", 5*time.Second); ok {
		writeJSON(w, http.StatusOK, data)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": settings.Version})
}

func (s *Server) handleLastUpdateDB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if data, ok := s.forwardBackendJSON(r.Context(), "/lastupdatedb", 5*time.Second); ok {
		writeJSON(w, http.StatusOK, data)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lastupdatedb": ""})
}

func (s *Server) forwardBackendJSON(ctx context.Context, path string, timeout time.Duration) (any, bool) {
	settings := s.store.Get()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settings.BaseURL+path, nil)
	if err != nil {
		s.logger.Warningf("backend %s request error: %v", path, err)
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Warningf("backend %s error: %v", path, err)
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.logger.Warningf("backend %s HTTP %d", path, resp.StatusCode)
		return nil, false
	}
	var data any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		s.logger.Warningf("backend %s JSON error: %v", path, err)
		return nil, false
	}
	return data, true
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func badRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}

func internalError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
}

func textStatus(status int) string {
	return fmt.Sprintf("%d %s", status, http.StatusText(status))
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key, x-api-key, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
}
