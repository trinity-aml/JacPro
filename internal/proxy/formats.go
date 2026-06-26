package proxy

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultTrackerName = "JacRed"

var looseInfoHashRE = regexp.MustCompile(`(?i)btih:([a-f0-9]+)`)

func resultToJackettJSON(torrent map[string]any) map[string]any {
	title := safeGet(torrent, "Title", "title", "name")
	info := mapValue(torrent["info"])
	category := torrent["Category"]
	if category == nil {
		var types any
		if info != nil {
			types = info["types"]
		}
		if types == nil {
			types = torrent["types"]
		}
		category = typesToCategories(types)
	}
	details := safeGet(torrent, "Details", "url")
	var detailsValue any
	if details != "" {
		detailsValue = details
	}
	var infoValue any
	if info != nil {
		infoValue = info
	}
	return map[string]any{
		"Tracker":      firstNonEmpty(safeGet(torrent, "Tracker", "TrackerId", "tracker"), defaultTrackerName),
		"Details":      detailsValue,
		"Title":        title,
		"Size":         toInt(firstNonEmpty(safeGet(torrent, "Size", "size"), "0")),
		"PublishDate":  firstPresent(torrent, "PublishDate", "createTime"),
		"Category":     category,
		"CategoryDesc": torrent["CategoryDesc"],
		"Seeders":      toInt(firstNonEmpty(safeGet(torrent, "Seeders", "sid"), "0")),
		"Peers":        toInt(firstNonEmpty(safeGet(torrent, "Peers", "pir"), "0")),
		"MagnetUri":    safeGet(torrent, "MagnetUri", "Magnet", "magnet"),
		"languages":    torrent["languages"],
		"info":         infoValue,
	}
}

func proxyBaseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

func wrapInXML(itemsXML string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed">
    <channel>
        <title>JacPro</title>
        <description>Torznab and Jackett gateway for JacRed</description>
        <link>http://localhost:5002/</link>
        <language>en-us</language>
        <category>search</category>
        %s
    </channel>
</rss>`, itemsXML)
}

func getCapsXML(r *http.Request) string {
	base := html.EscapeString(proxyBaseURL(r))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<caps>
  <server version="1.0" title="JacPro" strapline="Torznab and Jackett gateway for JacRed" email="info@localhost" url="%s/api/v2.0/indexers/all/results/torznab/api"/>
  <limits max="1000" default="100"/>
  <searching>
    <search available="yes" supportedParams="q,imdbid"/>
    <tv-search available="yes" supportedParams="q,imdbid,tvdbid,season,ep"/>
    <movie-search available="yes" supportedParams="q,imdbid"/>
  </searching>
  <categories>
    <category id="2000" name="Movies"/>
    <category id="5000" name="TV"/>
    <category id="5070" name="TV/Anime"/>
  </categories>
</caps>`, base)
}

func getIndexersXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<indexers>
  <indexer id="all" configured="true">
    <title>JacRed (all trackers)</title>
    <description>Aggregated JacRed search across all configured trackers</description>
    <link>https://github.com/jacred-fdb/jacred</link>
    <language>ru-RU</language>
    <type>public</type>
  </indexer>
</indexers>`
}

func resolveItemCategory(torrent map[string]any, assignedCat, catParam string) string {
	if assignedCat != "" {
		return assignedCat
	}
	raw := torrent["Category"]
	if raw == nil {
		raw = torrent["category"]
	}
	if list := values(raw); len(list) > 0 {
		if first := strings.TrimSpace(valueString(list[0])); first != "" {
			return first
		}
	}
	if catParam != "" {
		first := strings.TrimSpace(strings.Split(catParam, ",")[0])
		if first != "" {
			return first
		}
	}
	return "2000"
}

func languageAttrs(torrent map[string]any, title string) (string, string) {
	for _, lang := range stringList(torrent["languages"]) {
		switch strings.ToLower(lang) {
		case "rus":
			return "ru-RU", "ru"
		case "eng":
			return "en-US", "en"
		}
	}
	if hasCyrillic(title) {
		return "ru-RU", "ru"
	}
	if hasLatin(title) && !hasCyrillic(title) {
		return "en-US", "en"
	}
	return "ru-RU", "ru"
}

func torrentToXMLItem(torrent map[string]any, assignedCat, catParam string, settings Settings) string {
	title := safeGet(torrent, "Title", "title", "name")
	if title == "" {
		title = "Unknown"
	}
	var voices []string
	if info := mapValue(torrent["info"]); info != nil {
		voices = stringList(info["voices"])
	}
	if len(voices) == 0 {
		voices = stringList(torrent["voices"])
	}
	displayTitle := title
	if settings.EnrichTitles {
		if len(voices) > 0 {
			displayTitle = fmt.Sprintf("%s | [%s].rus", title, strings.Join(voices, " "))
		} else {
			displayTitle = title + " | [].rus"
		}
	}

	langTag, langCode := languageAttrs(torrent, title)
	magnetURL := safeGet(torrent, "MagnetUri", "Magnet", "Link")
	if magnetURL == "" {
		magnetURL = safeGet(torrent, "Details")
	}
	size := toInt(firstNonEmpty(safeGet(torrent, "Size"), "0"))
	indexerName := firstNonEmpty(safeGet(torrent, "Tracker", "TrackerId", "Indexer"), defaultTrackerName)
	seeders := toInt(firstNonEmpty(safeGet(torrent, "Seeders"), "0"))
	leechers := toInt(firstNonEmpty(safeGet(torrent, "Peers", "Leechers"), "0"))
	peersTotal := seeders
	if leechers > 0 {
		peersTotal = seeders + leechers
	}
	itemCat := resolveItemCategory(torrent, assignedCat, catParam)
	infohash := safeGet(torrent, "InfoHash", "Hash")
	if infohash == "" {
		if match := looseInfoHashRE.FindStringSubmatch(magnetURL); len(match) == 2 {
			infohash = match[1]
		}
	}

	safeTitle := html.EscapeString(displayTitle)
	safeLink := html.EscapeString(magnetURL)
	safeIndexer := html.EscapeString(indexerName)
	guid := infohash
	if guid == "" {
		sum := md5.Sum([]byte(safeTitle))
		guid = hex.EncodeToString(sum[:])
	}

	return fmt.Sprintf(`
    <item>
        <title>%s</title>
        <guid isPermaLink="false">%s</guid>
        <link>%s</link>
        <pubDate>%s</pubDate>
        <size>%d</size>
        <enclosure url="%s" length="%d" type="application/x-bittorrent" />

        <category>%s</category>
        <indexer id="%s">%s</indexer>
        <jackettindexer id="%s">%s</jackettindexer>

        <torznab:attr name="magneturl" value="%s" />
        <torznab:attr name="infohash" value="%s" />
        <torznab:attr name="seeders" value="%d" />
        <torznab:attr name="peers" value="%d" />
        <torznab:attr name="site" value="%s" />
        <torznab:attr name="category" value="%s" />

        <torznab:attr name="language" value="%s" />
        <torznab:attr name="lang" value="%s" />
    </item>`,
		safeTitle,
		html.EscapeString(guid),
		safeLink,
		time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000"),
		size,
		safeLink,
		size,
		html.EscapeString(itemCat),
		safeIndexer,
		safeIndexer,
		safeIndexer,
		safeIndexer,
		safeLink,
		html.EscapeString(strings.ToUpper(infohash)),
		seeders,
		peersTotal,
		safeIndexer,
		html.EscapeString(itemCat),
		langTag,
		langCode,
	)
}
