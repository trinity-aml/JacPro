package proxy

import "testing"

func TestSplitBilingualQuery(t *testing.T) {
	ru, en := splitBilingualQuery("Матрица / The Matrix")
	if ru != "Матрица" || en != "The Matrix" {
		t.Fatalf("unexpected split: ru=%q en=%q", ru, en)
	}

	ru, en = splitBilingualQuery("The Matrix / Матрица")
	if ru != "Матрица" || en != "The Matrix" {
		t.Fatalf("unexpected reverse split: ru=%q en=%q", ru, en)
	}
}

func TestBuildQueryVariantsStripYear(t *testing.T) {
	settings := Settings{StripTrailingYear: true}
	variants := buildQueryVariants("The Matrix 1999", "", "", settings)
	if len(variants) != 2 {
		t.Fatalf("expected stripped and original variants, got %#v", variants)
	}
	if variants[0] != "The Matrix" || variants[1] != "The Matrix 1999" {
		t.Fatalf("unexpected variants: %#v", variants)
	}
}

func TestMergeTorrentListsDedupesByInfohash(t *testing.T) {
	first := map[string]any{"Title": "A", "InfoHash": "ABCDEF"}
	second := map[string]any{"Title": "B", "InfoHash": "abcdef"}
	merged := mergeTorrentLists([]map[string]any{first}, []map[string]any{second})
	if len(merged) != 1 {
		t.Fatalf("expected one deduped result, got %d", len(merged))
	}
	if merged[0]["Title"] != "A" {
		t.Fatalf("expected first result to win, got %#v", merged[0])
	}
}
