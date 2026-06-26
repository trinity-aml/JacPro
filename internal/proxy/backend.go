package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Backend struct {
	store  *SettingsStore
	logger *Logger
	client *http.Client
}

func NewBackend(store *SettingsStore, logger *Logger) *Backend {
	return &Backend{
		store:  store,
		logger: logger,
		client: &http.Client{},
	}
}

func (b *Backend) getJSON(ctx context.Context, path string, params url.Values, label string, settings Settings) any {
	timeout := time.Duration(settings.RequestTimeout) * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := settings.BaseURL + path
	u, err := url.Parse(endpoint)
	if err != nil {
		b.logger.Errorf("[BACKEND] %s invalid URL %s: %v", label, endpoint, err)
		return nil
	}
	query := u.Query()
	for key, values := range params {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		b.logger.Errorf("[BACKEND] %s request error: %v", label, err)
		return nil
	}

	logParams := url.Values{}
	for key, values := range params {
		for _, value := range values {
			if strings.EqualFold(key, "apikey") {
				value = "***"
			}
			logParams.Add(key, value)
		}
	}
	b.logger.Infof("[BACKEND] %s: %s", label, endpoint)
	b.logger.Debugf("[BACKEND] params: %s", logParams.Encode())

	resp, err := b.client.Do(req)
	if err != nil {
		b.logger.Errorf("[BACKEND] %s error: %v", label, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b.logger.Errorf("[BACKEND] %s HTTP %d", label, resp.StatusCode)
		return nil
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	var data any
	if err := decoder.Decode(&data); err != nil {
		b.logger.Errorf("[BACKEND] %s JSON error: %v", label, err)
		return nil
	}
	b.logger.Infof("[BACKEND] %s OK", label)
	return data
}

func apiParams(apikey string, settings Settings) url.Values {
	params := url.Values{}
	key := apikey
	if key == "" {
		key = settings.APIKey
	}
	if key != "" {
		params.Set("apikey", key)
	}
	return params
}

func parseV2Results(data any) []map[string]any {
	if obj := mapValue(data); obj != nil {
		return resultList(obj["Results"])
	}
	return resultList(data)
}

func resultList(data any) []map[string]any {
	values, ok := data.([]any)
	if !ok {
		return nil
	}
	results := make([]map[string]any, 0, len(values))
	for _, item := range values {
		if m := mapValue(item); m != nil {
			results = append(results, m)
		}
	}
	return results
}

type FetchV2Options struct {
	APIKey        string
	Query         string
	Title         string
	TitleOriginal string
	Year          int
	Tracker       string
	IsSerial      int
	Genres        string
	Categories    []string
	Label         string
	Settings      Settings
}

func (b *Backend) fetchV2(ctx context.Context, opts FetchV2Options) []map[string]any {
	label := opts.Label
	if label == "" {
		label = "v2"
	}
	params := apiParams(opts.APIKey, opts.Settings)
	if opts.Tracker != "" {
		params.Set("Tracker", opts.Tracker)
	}
	if opts.Query != "" {
		params.Set("query", opts.Query)
		params.Set("Query", opts.Query)
	}
	if opts.Title != "" {
		params.Set("title", opts.Title)
	}
	if opts.TitleOriginal != "" {
		params.Set("title_original", opts.TitleOriginal)
	}
	if opts.Year > 0 {
		params.Set("year", fmt.Sprintf("%d", opts.Year))
	}
	if opts.Genres != "" {
		params.Set("genres", opts.Genres)
	}
	for key, value := range categoryParamsFromList(opts.Categories) {
		params.Set(key, value)
	}
	if opts.IsSerial >= 0 {
		params.Set("is_serial", fmt.Sprintf("%d", opts.IsSerial))
	}
	if params.Get("query") == "" && params.Get("Query") == "" && params.Get("title") == "" {
		return nil
	}

	data := b.getJSON(ctx, "/api/v2.0/indexers/all/results", params, label, opts.Settings)
	results := parseV2Results(data)
	b.logger.Infof("[BACKEND] %s -> %d items", label, len(results))
	return results
}

type FetchV1Options struct {
	APIKey   string
	Search   string
	AltName  string
	Exact    bool
	Season   int
	Settings Settings
}

func (b *Backend) fetchV1(ctx context.Context, opts FetchV1Options) []map[string]any {
	if opts.Search == "" {
		return nil
	}
	params := apiParams(opts.APIKey, opts.Settings)
	params.Set("search", opts.Search)
	if opts.AltName != "" {
		params.Set("altname", opts.AltName)
	}
	if opts.Exact {
		params.Set("exact", "true")
	}
	if opts.Season > 0 {
		params.Set("season", fmt.Sprintf("%d", opts.Season))
	}

	data := b.getJSON(ctx, "/api/v1.0/torrents", params, "v1", opts.Settings)
	obj := mapValue(data)
	if obj == nil {
		return nil
	}
	results := make([]map[string]any, 0, len(obj))
	for _, item := range obj {
		if m := mapValue(item); m != nil {
			results = append(results, v1ItemToResult(m))
		}
	}
	b.logger.Infof("[BACKEND] v1 -> %d items", len(results))
	return results
}
