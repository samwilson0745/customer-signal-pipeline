package search

import "testing"

func TestBuildQueryDefaultsLimitWhenUnset(t *testing.T) {
	body := buildQuery(Query{})
	if body["size"] != DefaultLimit {
		t.Fatalf("expected default size %d, got %v", DefaultLimit, body["size"])
	}
	if body["from"] != 0 {
		t.Fatalf("expected default offset 0, got %v", body["from"])
	}
}

func TestBuildQueryClampsLimitToMax(t *testing.T) {
	body := buildQuery(Query{Limit: MaxLimit + 500})
	if body["size"] != MaxLimit {
		t.Fatalf("expected size clamped to %d, got %v", MaxLimit, body["size"])
	}
}

func TestBuildQueryHonorsExplicitLimitAndOffset(t *testing.T) {
	body := buildQuery(Query{Limit: 10, Offset: 20})
	if body["size"] != 10 {
		t.Fatalf("expected size 10, got %v", body["size"])
	}
	if body["from"] != 20 {
		t.Fatalf("expected from 20, got %v", body["from"])
	}
}

func TestBuildQueryUsesMatchAllWhenTextEmpty(t *testing.T) {
	body := buildQuery(Query{})
	must := body["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{})
	if _, ok := must[0]["match_all"]; !ok {
		t.Fatalf("expected match_all clause when text is empty, got %v", must[0])
	}
}
