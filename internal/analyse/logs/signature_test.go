package logs

import "testing"

func TestNormalizeSignature(t *testing.T) {
	line := "2025-01-01T12:34:56Z ERROR request_id=4f2a9b3c-7f1b-4a3b-9c21-1234567890ab failed in 1234ms for 10.1.2.3"
	normalized := normalizeSignature(line)
	if normalized == "" {
		t.Fatal("expected normalized signature")
	}
	if normalized == line {
		t.Fatal("expected normalized output to differ from input")
	}
	if containsRawUUID(normalized) {
		t.Fatalf("expected uuid to be normalized: %s", normalized)
	}
}

func TestNormalizeSignatureExtractsReasonPair(t *testing.T) {
	line := "2026-01-30 12:06:45,528 ERROR notification: queue_backlog_high (downstream webhook backlog)"
	normalized := normalizeSignature(line)
	if normalized != "notification: queue_backlog_high" {
		t.Fatalf("expected reason pair, got %q", normalized)
	}
}

func containsRawUUID(value string) bool {
	return uuidRe.MatchString(value)
}
