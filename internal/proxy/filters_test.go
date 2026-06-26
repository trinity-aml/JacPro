package proxy

import "testing"

func testLogger(t *testing.T) *Logger {
	t.Helper()
	logger, err := NewLogger(Settings{LogLevel: "CRITICAL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger
}

func TestFilterResultsByCategoryMatchesCategoryFamily(t *testing.T) {
	torrents := []map[string]any{
		{"Title": "movie", "Category": []any{2000}},
		{"Title": "series", "Category": []any{5000}},
	}
	filtered := filterResultsByCategory(torrents, "5070", testLogger(t))
	if len(filtered) != 1 || filtered[0]["Title"] != "series" {
		t.Fatalf("unexpected category filter result: %#v", filtered)
	}
}

func TestFilterResultsBySeasonEpisode(t *testing.T) {
	torrents := []map[string]any{
		{"Title": "Show S02E03 1080p"},
		{"Title": "Show S01E03 1080p"},
		{"Title": "Show S02E01-10 1080p"},
	}
	filtered := filterResultsBySeasonEpisode(torrents, 2, 3, true, testLogger(t))
	if len(filtered) != 2 {
		t.Fatalf("expected episode match and season pack range, got %#v", filtered)
	}
}

func TestFilterResultsByYearAllowsAdjacentYears(t *testing.T) {
	torrents := []map[string]any{
		{"Title": "same", "info": map[string]any{"relased": 2024}},
		{"Title": "adjacent", "info": map[string]any{"relased": 2023}},
		{"Title": "old", "info": map[string]any{"relased": 2020}},
	}
	filtered := filterResultsByYear(torrents, 2024, testLogger(t))
	if len(filtered) != 2 {
		t.Fatalf("unexpected year filter result: %#v", filtered)
	}
}
