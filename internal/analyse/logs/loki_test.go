package logs

import "testing"

func TestBuildLogQLIncludesDependencyAndRegex(t *testing.T) {
	query, notes, err := buildLogQL(`{service="api"}`, `(?i)(error|5\\d\\d)`, "postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query == `{service="api"}` {
		t.Fatalf("expected log filters in query: %s", query)
	}
	if len(notes) == 0 {
		t.Fatalf("expected notes for dependency/regex")
	}
}

func TestBuildLogQLSkipsInvalidRegex(t *testing.T) {
	query, notes, err := buildLogQL(`{job=~".+"}`, `[`, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query != `{job=~".+"}` {
		t.Fatalf("expected base selector when regex invalid: %s", query)
	}
	if len(notes) == 0 {
		t.Fatalf("expected notes for invalid regex")
	}
}
