package proxy

import (
	"regexp"
	"strings"
)

const (
	defaultTorznabLimit = 100
	maxTorznabLimit     = 1000
)

var (
	sxxexxRE      = regexp.MustCompile(`(?i)(?m)(^|[^0-9])s(?P<season>\d{1,2})[\s._-]*e(?P<episode>\d{1,3})([^0-9]|$)`)
	sxxexxRangeRE = regexp.MustCompile(`(?i)(?m)(^|[^0-9])s(?P<season>\d{1,2})[\s._-]*e(?P<ep_start>\d{1,3})\s*-\s*(?P<ep_end>\d{1,3})([^0-9]|$)`)
	nxnnRE        = regexp.MustCompile(`(?i)(?m)(^|[^0-9])(?P<season>\d{1,2})x(?P<episode>\d{1,3})([^0-9]|$)`)
	bracketSRE    = regexp.MustCompile(`(?i)\[s(?P<season>\d{1,2})\]`)
	seasonWordRE  = regexp.MustCompile(`(?i)(?:season|сезон)\s*(?P<season>\d{1,2})\b`)
	sxxPackRE     = regexp.MustCompile(`(?i)(^|[^0-9])s(?P<season>\d{1,2})([^\d]|\s*$)`)
)

type parsedRelease struct {
	Season       int
	Episode      int
	HasEpisode   bool
	IsSeasonPack bool
}

func torrentReleaseYear(torrent map[string]any) (int, bool) {
	if info := mapValue(torrent["info"]); info != nil {
		if year, ok := toIntOK(info["relased"]); ok {
			return year, true
		}
	}
	if year, ok := toIntOK(torrent["relased"]); ok {
		return year, true
	}
	return 0, false
}

func torrentMatchesYear(torrent map[string]any, year int) bool {
	rel, ok := torrentReleaseYear(torrent)
	if !ok {
		return true
	}
	return rel == year || rel == year-1 || rel == year+1
}

func filterResultsByYear(torrents []map[string]any, year int, logger *Logger) []map[string]any {
	if year <= 0 {
		return torrents
	}
	filtered := make([]map[string]any, 0, len(torrents))
	for _, torrent := range torrents {
		if torrentMatchesYear(torrent, year) {
			filtered = append(filtered, torrent)
		}
	}
	if len(filtered) < len(torrents) {
		logger.Infof("[TORZNAB] year filter %d: %d -> %d", year, len(torrents), len(filtered))
	}
	return filtered
}

func paginateResults(torrents []map[string]any, limit, offset int, hasLimit, hasOffset bool, logger *Logger) []map[string]any {
	off := offset
	if off < 0 {
		off = 0
	}
	if !hasLimit && off == 0 && !hasOffset {
		return torrents
	}
	lim := defaultTorznabLimit
	if hasLimit {
		lim = limit
		if lim < 0 {
			lim = 0
		}
		if lim > maxTorznabLimit {
			lim = maxTorznabLimit
		}
	}
	if off >= len(torrents) {
		logger.Infof("[TORZNAB] pagination offset=%d limit=%d: %d -> 0", off, lim, len(torrents))
		return nil
	}
	end := off + lim
	if end > len(torrents) {
		end = len(torrents)
	}
	page := torrents[off:end]
	if off > 0 || len(page) < len(torrents) {
		logger.Infof("[TORZNAB] pagination offset=%d limit=%d: %d -> %d", off, lim, len(torrents), len(page))
	}
	return page
}

func torrentSeasonsSet(torrent map[string]any) map[int]bool {
	seasons := map[int]bool{}
	add := func(raw any) {
		for _, value := range values(raw) {
			if season, ok := toIntOK(value); ok && season > 0 {
				seasons[season] = true
			}
		}
	}
	add(torrent["seasons"])
	if info := mapValue(torrent["info"]); info != nil {
		add(info["seasons"])
	}
	return seasons
}

func parseReleaseTitle(title string) (parsedRelease, bool) {
	if strings.TrimSpace(title) == "" {
		return parsedRelease{}, false
	}
	if match := namedMatch(sxxexxRangeRE, title); match != nil {
		return parsedRelease{
			Season:       toInt(match["season"]),
			Episode:      toInt(match["ep_start"]),
			HasEpisode:   true,
			IsSeasonPack: true,
		}, true
	}
	if match := namedMatch(sxxexxRE, title); match != nil {
		return parsedRelease{
			Season:     toInt(match["season"]),
			Episode:    toInt(match["episode"]),
			HasEpisode: true,
		}, true
	}
	if match := namedMatch(nxnnRE, title); match != nil {
		return parsedRelease{
			Season:     toInt(match["season"]),
			Episode:    toInt(match["episode"]),
			HasEpisode: true,
		}, true
	}
	if match := namedMatch(bracketSRE, title); match != nil {
		return parsedRelease{Season: toInt(match["season"]), IsSeasonPack: true}, true
	}
	if match := namedMatch(seasonWordRE, title); match != nil {
		return parsedRelease{Season: toInt(match["season"]), IsSeasonPack: true}, true
	}
	if match := namedMatch(sxxPackRE, title); match != nil {
		return parsedRelease{Season: toInt(match["season"]), IsSeasonPack: true}, true
	}
	return parsedRelease{}, false
}

func namedMatch(re *regexp.Regexp, text string) map[string]any {
	match := re.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	out := map[string]any{}
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" && i < len(match) {
			out[name] = match[i]
		}
	}
	return out
}

func episodeInRange(title string, season, episode int) bool {
	matches := sxxexxRangeRE.FindAllStringSubmatch(title, -1)
	names := sxxexxRangeRE.SubexpNames()
	for _, match := range matches {
		fields := map[string]string{}
		for i, name := range names {
			if i > 0 && name != "" && i < len(match) {
				fields[name] = match[i]
			}
		}
		if toInt(fields["season"]) != season {
			continue
		}
		start := toInt(fields["ep_start"])
		end := toInt(fields["ep_end"])
		if start <= episode && episode <= end {
			return true
		}
	}
	return false
}

func releaseMatchesSeasonEpisode(title string, season int, episode int, hasEpisode bool) bool {
	if hasEpisode && episodeInRange(title, season, episode) {
		return true
	}
	parsed, ok := parseReleaseTitle(title)
	if !ok || parsed.Season != season {
		return false
	}
	if !hasEpisode {
		return true
	}
	if parsed.IsSeasonPack || !parsed.HasEpisode {
		return true
	}
	return parsed.Episode == episode
}

func titleCandidates(torrent map[string]any) []string {
	var titles []string
	if title := safeGet(torrent, "Title", "title", "name"); title != "" {
		titles = append(titles, title)
	}
	if info := mapValue(torrent["info"]); info != nil {
		for _, key := range []string{"name", "originalname"} {
			alt := valueString(info[key])
			if alt == "" {
				continue
			}
			exists := false
			for _, title := range titles {
				if title == alt {
					exists = true
					break
				}
			}
			if !exists {
				titles = append(titles, alt)
			}
		}
	}
	return titles
}

func torrentMatchesSeasonEpisode(torrent map[string]any, season int, episode int, hasEpisode bool) bool {
	meta := torrentSeasonsSet(torrent)
	if len(meta) > 0 {
		if !meta[season] {
			return false
		}
		if !hasEpisode {
			return true
		}
		for _, title := range titleCandidates(torrent) {
			if releaseMatchesSeasonEpisode(title, season, episode, true) {
				return true
			}
		}
		return false
	}
	for _, title := range titleCandidates(torrent) {
		if releaseMatchesSeasonEpisode(title, season, episode, hasEpisode) {
			return true
		}
	}
	return false
}

func filterResultsBySeasonEpisode(torrents []map[string]any, season int, episode int, hasEpisode bool, logger *Logger) []map[string]any {
	if season <= 0 {
		return torrents
	}
	filtered := make([]map[string]any, 0, len(torrents))
	for _, torrent := range torrents {
		if torrentMatchesSeasonEpisode(torrent, season, episode, hasEpisode) {
			filtered = append(filtered, torrent)
		}
	}
	if len(filtered) < len(torrents) {
		ep := "*"
		if hasEpisode {
			ep = valueString(episode)
		}
		logger.Infof("[TORZNAB] season/ep filter S%dE%s: %d -> %d", season, ep, len(torrents), len(filtered))
	}
	return filtered
}
