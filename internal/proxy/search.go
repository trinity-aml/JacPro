package proxy

import (
	"context"
	"sync"
)

type SearchOptions struct {
	APIKey        string
	Query         string
	Title         string
	TitleOriginal string
	Year          int
	Tracker       string
	IsSerial      int
	Genres        string
	Categories    []string
	Season        int
	MergeV1       *bool
	CardMode      *bool
	Settings      Settings
}

type v1Pair struct {
	Search  string
	AltName string
}

func buildQueryVariants(query, titleRU, titleEN string, settings Settings) []string {
	var variants []string
	add := func(term string) {
		if term == "" {
			return
		}
		for _, existing := range variants {
			if existing == term {
				return
			}
		}
		variants = append(variants, term)
	}
	if query != "" {
		if settings.StripTrailingYear {
			add(stripTrailingYear(query, true))
		}
		add(query)
	}
	add(titleRU)
	add(titleEN)
	return variants
}

func v1SearchPairs(query, titleRU, titleEN string, settings Settings) []v1Pair {
	var pairs []v1Pair
	seen := map[v1Pair]bool{}
	add := func(search, alt string) {
		if search == "" {
			return
		}
		pair := v1Pair{Search: search, AltName: alt}
		if seen[pair] {
			return
		}
		seen[pair] = true
		pairs = append(pairs, pair)
	}

	if titleRU != "" && titleEN != "" {
		add(titleEN, titleRU)
		add(titleRU, titleEN)
	} else if titleRU != "" {
		add(titleRU, titleEN)
	} else if titleEN != "" {
		add(titleEN, titleRU)
	}

	for _, term := range buildQueryVariants(query, titleRU, titleEN, settings) {
		add(term, "")
		if titleRU != "" && !contains(term, titleRU) {
			add(term, titleRU)
		}
		if titleEN != "" && !contains(term, titleEN) {
			add(term, titleEN)
		}
	}
	return pairs
}

func contains(text, part string) bool {
	return len(part) == 0 || len(text) >= len(part) && (text == part || index(text, part) >= 0)
}

func index(text, part string) int {
	for i := 0; i+len(part) <= len(text); i++ {
		if text[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func isCardMetadataSearch(title, titleOriginal string, isSerial *int, categories []string, genres string) bool {
	if title != "" || titleOriginal != "" {
		return true
	}
	if isSerial != nil && *isSerial >= 0 {
		return true
	}
	if len(categories) > 0 || genres != "" {
		return true
	}
	return false
}

func (b *Backend) SearchCombined(ctx context.Context, opts SearchOptions) []map[string]any {
	settings := opts.Settings
	mergeV1 := settings.MergeV1
	if opts.MergeV1 != nil {
		mergeV1 = *opts.MergeV1
	}
	cardMode := false
	if opts.CardMode != nil {
		cardMode = *opts.CardMode
	} else {
		var serial *int
		if opts.IsSerial >= 0 {
			serial = &opts.IsSerial
		}
		cardMode = isCardMetadataSearch(opts.Title, opts.TitleOriginal, serial, opts.Categories, opts.Genres)
	}

	titleRU := opts.Title
	titleEN := opts.TitleOriginal
	if titleRU == "" && titleEN == "" {
		titleRU, titleEN = splitBilingualQuery(opts.Query)
	}

	var tasks []func() []map[string]any
	if cardMode {
		fetchOpts := FetchV2Options{
			APIKey:        opts.APIKey,
			Query:         opts.Query,
			Title:         titleRU,
			TitleOriginal: titleEN,
			Year:          opts.Year,
			Tracker:       opts.Tracker,
			IsSerial:      opts.IsSerial,
			Genres:        opts.Genres,
			Categories:    opts.Categories,
			Label:         "v2-card",
			Settings:      settings,
		}
		tasks = append(tasks, func() []map[string]any {
			return b.fetchV2(ctx, fetchOpts)
		})
	} else {
		for _, term := range buildQueryVariants(opts.Query, titleRU, titleEN, settings) {
			label := "v2-fuzzy:" + term
			if len(label) > len("v2-fuzzy:")+32 {
				label = label[:len("v2-fuzzy:")+32]
			}
			fetchOpts := FetchV2Options{
				APIKey:   opts.APIKey,
				Query:    term,
				Tracker:  opts.Tracker,
				IsSerial: opts.IsSerial,
				Label:    label,
				Settings: settings,
			}
			tasks = append(tasks, func() []map[string]any {
				return b.fetchV2(ctx, fetchOpts)
			})
		}
	}

	if mergeV1 {
		for _, pair := range v1SearchPairs(opts.Query, titleRU, titleEN, settings) {
			fetchOpts := FetchV1Options{
				APIKey:   opts.APIKey,
				Search:   pair.Search,
				AltName:  pair.AltName,
				Season:   opts.Season,
				Settings: settings,
			}
			tasks = append(tasks, func() []map[string]any {
				return b.fetchV1(ctx, fetchOpts)
			})
		}
	}

	batches := runSearchTasks(tasks)
	merged := mergeTorrentLists(batches...)
	mode := "fuzzy"
	if cardMode {
		mode = "card"
	}
	b.logger.Infof("[BACKEND] combined %d unique (mode=%s v1=%v)", len(merged), mode, mergeV1)
	return merged
}

func runSearchTasks(tasks []func() []map[string]any) [][]map[string]any {
	batches := make([][]map[string]any, len(tasks))
	var wg sync.WaitGroup
	wg.Add(len(tasks))
	for i, task := range tasks {
		go func(i int, task func() []map[string]any) {
			defer wg.Done()
			batches[i] = task()
		}(i, task)
	}
	wg.Wait()
	return batches
}
