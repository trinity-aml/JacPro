package proxy

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	cyrillicRE         = regexp.MustCompile(`[\x{0400}-\x{04FF}]`)
	latinRE            = regexp.MustCompile(`[a-zA-Z]`)
	trailingYearRE     = regexp.MustCompile(`\s*\b(19|20)\d{2}\b\s*$`)
	infoHashFromMagnet = regexp.MustCompile(`(?i)btih:([a-f0-9]{40})`)
)

var typeToCategory = map[string]int{
	"movie":      2000,
	"multfilm":   2000,
	"documovie":  2000,
	"serial":     5000,
	"multserial": 5000,
	"docuserial": 5000,
	"tvshow":     5000,
	"anime":      5070,
}

func safeGet(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if text := valueString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func valueString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

func toInt(value any) int {
	n, _ := toIntOK(value)
	return n
}

func toIntOK(value any) (int, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i), true
		}
		f, err := v.Float64()
		if err == nil {
			return int(f), true
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

func mapValue(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func stringList(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if strings.TrimSpace(item) != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text := strings.TrimSpace(valueString(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	default:
		if text := strings.TrimSpace(valueString(v)); text != "" {
			return []string{text}
		}
	}
	return nil
}

func hasCyrillic(text string) bool {
	return cyrillicRE.MatchString(text)
}

func hasLatin(text string) bool {
	return latinRE.MatchString(text)
}

func stripTrailingYear(query string, enabled bool) string {
	if query == "" || !enabled {
		return query
	}
	cleaned := strings.TrimSpace(trailingYearRE.ReplaceAllString(query, ""))
	if cleaned == "" {
		return query
	}
	return cleaned
}

func splitBilingualQuery(query string) (string, string) {
	q := strings.TrimSpace(query)
	if q == "" || !strings.Contains(q, " / ") {
		return "", ""
	}
	parts := strings.SplitN(q, " / ", 2)
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	switch {
	case hasCyrillic(left) && hasLatin(right):
		return left, right
	case hasLatin(left) && hasCyrillic(right):
		return right, left
	default:
		return left, right
	}
}

func typesToCategories(types any) []int {
	values := stringList(types)
	if len(values) == 0 {
		return nil
	}
	seen := map[int]bool{}
	cats := make([]int, 0, len(values))
	for _, tag := range values {
		if cid, ok := typeToCategory[tag]; ok && !seen[cid] {
			seen[cid] = true
			cats = append(cats, cid)
		}
	}
	if len(cats) == 0 {
		return []int{2000}
	}
	return cats
}

func infoHashFromTorrent(torrent map[string]any) string {
	if ih := safeGet(torrent, "InfoHash", "Hash"); ih != "" {
		ih = strings.ToLower(strings.TrimSpace(ih))
		if len(ih) > 40 {
			ih = ih[:40]
		}
		return ih
	}
	magnet := safeGet(torrent, "MagnetUri", "Magnet", "magnet", "Link", "link")
	match := infoHashFromMagnet.FindStringSubmatch(magnet)
	if len(match) == 2 {
		return strings.ToLower(match[1])
	}
	return ""
}

func torrentDedupeKey(torrent map[string]any) string {
	if ih := infoHashFromTorrent(torrent); ih != "" {
		return "h:" + ih
	}
	title := safeGet(torrent, "Title", "title", "name")
	magnet := safeGet(torrent, "MagnetUri", "Magnet", "magnet")
	sum := md5.Sum([]byte(title + "|" + magnet))
	return "x:" + hex.EncodeToString(sum[:])
}

func mergeTorrentLists(batches ...[]map[string]any) []map[string]any {
	seen := map[string]bool{}
	var merged []map[string]any
	for _, batch := range batches {
		for _, torrent := range batch {
			key := torrentDedupeKey(torrent)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, torrent)
		}
	}
	return merged
}
