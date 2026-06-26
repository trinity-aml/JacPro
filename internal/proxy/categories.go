package proxy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func normalizeCategoryID(value any) (int, bool) {
	raw := strings.TrimSpace(valueString(value))
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	if raw == "" {
		return 0, false
	}
	id, err := strconv.Atoi(raw)
	return id, err == nil
}

func parseTorznabCategories(catParam string) map[int]bool {
	cats := map[int]bool{}
	for _, part := range strings.Split(catParam, ",") {
		if id, ok := normalizeCategoryID(part); ok {
			cats[id] = true
		}
	}
	return cats
}

func isSerialFromSearch(t, catParam string) int {
	switch t {
	case "moviesearch":
		return 1
	case "tvsearch":
		return 2
	default:
		return -1
	}
}

func torrentMatchesTorznabCategories(torrent map[string]any, wanted map[int]bool) bool {
	if len(wanted) == 0 {
		return true
	}
	hasMovie := false
	hasTV := false
	for cat := range wanted {
		if cat >= 2000 && cat < 3000 {
			hasMovie = true
		}
		if cat >= 5000 && cat < 6000 {
			hasTV = true
		}
	}
	if hasMovie && hasTV {
		return true
	}

	raw, ok := torrent["Category"]
	if !ok {
		raw = torrent["category"]
	}
	var itemCats []int
	for _, value := range values(raw) {
		if id, ok := normalizeCategoryID(value); ok {
			itemCats = append(itemCats, id)
		}
	}
	if len(itemCats) == 0 {
		return true
	}

	for wantedCat := range wanted {
		base := (wantedCat / 1000) * 1000
		for _, itemCat := range itemCats {
			if itemCat >= base && itemCat < base+1000 {
				return true
			}
		}
	}
	return false
}

func filterResultsByCategory(torrents []map[string]any, catParam string, logger *Logger) []map[string]any {
	wanted := parseTorznabCategories(catParam)
	if len(wanted) == 0 {
		return torrents
	}
	filtered := make([]map[string]any, 0, len(torrents))
	for _, torrent := range torrents {
		if torrentMatchesTorznabCategories(torrent, wanted) {
			filtered = append(filtered, torrent)
		}
	}
	if len(filtered) < len(torrents) {
		logger.Infof("[TORZNAB] category filter %v: %d -> %d", wanted, len(torrents), len(filtered))
	}
	return filtered
}

func categoryParamsFromList(categories []string) map[string]string {
	params := map[string]string{}
	for i, cat := range categories {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			continue
		}
		params[fmt.Sprintf("Category[%d]", i)] = cat
	}
	return params
}

func categoriesFromValues(values url.Values) []string {
	var cats []string
	for _, value := range values["Category[]"] {
		if strings.TrimSpace(value) != "" {
			cats = append(cats, strings.TrimSpace(value))
		}
	}
	if len(cats) == 0 {
		for key, list := range values {
			if !strings.HasPrefix(key, "Category[") || key == "Category[]" {
				continue
			}
			for _, value := range list {
				if strings.TrimSpace(value) != "" {
					cats = append(cats, strings.TrimSpace(value))
				}
			}
		}
	}
	if len(cats) > 0 {
		return cats
	}
	raw := values.Get("cat")
	if raw == "" {
		raw = values.Get("Category")
	}
	for _, part := range strings.Split(raw, ",") {
		if strings.TrimSpace(part) != "" {
			cats = append(cats, strings.TrimSpace(part))
		}
	}
	return cats
}

func v1ItemToResult(item map[string]any) map[string]any {
	info := map[string]any{
		"name":         item["name"],
		"originalname": item["originalname"],
		"voices":       item["voices"],
		"types":        item["types"],
		"seasons":      item["seasons"],
		"relased":      item["relased"],
	}
	return map[string]any{
		"Tracker":   firstPresent(item, "tracker", "trackerName"),
		"Title":     item["title"],
		"Size":      toInt(item["size"]),
		"Seeders":   toInt(item["sid"]),
		"Peers":     toInt(item["pir"]),
		"MagnetUri": item["magnet"],
		"Details":   item["url"],
		"Category":  typesToCategories(item["types"]),
		"info":      info,
	}
}

func values(raw any) []any {
	switch v := raw.(type) {
	case nil:
		return nil
	case []any:
		return v
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out
	default:
		return []any{raw}
	}
}

func firstPresent(item map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := item[key]; ok && valueString(value) != "" {
			return value
		}
	}
	return nil
}
