package proxy

import (
	"net/url"
	"strconv"
	"strings"
)

type TorznabSearchParams struct {
	Query      string
	Season     int
	Episode    int
	HasSeason  bool
	HasEpisode bool
	Year       int
	Limit      int
	Offset     int
	HasLimit   bool
	HasOffset  bool
	TVDBID     int
	IMDBID     string
	TVDBIDOnly bool
}

func optionalInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func seasonEpisodeFromValues(values url.Values) (season int, hasSeason bool, episode int, hasEpisode bool) {
	if value, ok := optionalInt(values.Get("season")); ok && value > 0 {
		season, hasSeason = value, true
	}
	epRaw := values.Get("ep")
	if epRaw == "" {
		epRaw = values.Get("episode")
	}
	if value, ok := optionalInt(epRaw); ok && value > 0 {
		episode, hasEpisode = value, true
	}
	return season, hasSeason, episode, hasEpisode
}

func limitOffsetFromValues(values url.Values) (limit int, hasLimit bool, offset int, hasOffset bool) {
	if value, ok := optionalInt(values.Get("limit")); ok && value > 0 {
		limit, hasLimit = value, true
	}
	if value, ok := optionalInt(values.Get("offset")); ok {
		if value < 0 {
			value = 0
		}
		offset, hasOffset = value, true
	}
	return limit, hasLimit, offset, hasOffset
}

func yearFromValues(values url.Values) int {
	if year, ok := optionalInt(values.Get("year")); ok && year > 0 {
		return year
	}
	return 0
}

func normalizeIMDBID(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "tt") || strings.HasPrefix(value, "kp") {
		return value
	}
	if _, err := strconv.Atoi(value); err == nil {
		return "tt" + value
	}
	return value
}

func resolveTorznabQuery(values url.Values, logger *Logger) string {
	query := strings.TrimSpace(values.Get("q"))
	if query == "" {
		query = strings.TrimSpace(values.Get("Query"))
	}
	if query != "" {
		return query
	}
	if imdbid := values.Get("imdbid"); imdbid != "" {
		normalized := normalizeIMDBID(imdbid)
		if normalized != "" {
			logger.Infof("[TORZNAB] resolved imdbid=%q -> q=%q", imdbid, normalized)
			return normalized
		}
	}
	return ""
}

func torznabSearchParamsFromValues(values url.Values, logger *Logger) TorznabSearchParams {
	season, hasSeason, episode, hasEpisode := seasonEpisodeFromValues(values)
	limit, hasLimit, offset, hasOffset := limitOffsetFromValues(values)
	query := resolveTorznabQuery(values, logger)
	imdbid := normalizeIMDBID(values.Get("imdbid"))
	tvdbid, hasTVDBID := optionalInt(firstNonEmpty(values.Get("tvdbid"), values.Get("rid")))
	tvdbidOnly := hasTVDBID && tvdbid > 0 && query == ""
	if tvdbidOnly {
		logger.Warningf("[TORZNAB] tvdbid=%d without q/imdbid - JacRed has no TVDB lookup; empty result", tvdbid)
	}
	return TorznabSearchParams{
		Query:      query,
		Season:     season,
		Episode:    episode,
		HasSeason:  hasSeason,
		HasEpisode: hasEpisode,
		Year:       yearFromValues(values),
		Limit:      limit,
		Offset:     offset,
		HasLimit:   hasLimit,
		HasOffset:  hasOffset,
		TVDBID:     tvdbid,
		IMDBID:     imdbid,
		TVDBIDOnly: tvdbidOnly,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
