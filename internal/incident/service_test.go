package incident

import (
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	store := NewStoreWithDir(t.TempDir())
	service, err := NewService(store)
	if err != nil {
		t.Fatalf("service init failed: %v", err)
	}
	t.Setenv("USER", "root")
	t.Setenv("SUDO_USER", "sanjana")
	base := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	counter := 0
	service.now = func() time.Time {
		tick := base.Add(time.Duration(counter) * time.Minute)
		counter++
		return tick
	}
	return service
}

func TestServiceLifecycle(t *testing.T) {
	service := newTestService(t)
	incident, _, err := service.Start("order_api", P1, "Checkout timeouts", "")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if incident.State != StateStarted {
		t.Fatalf("expected state started, got %s", incident.State)
	}
	incident, _, err = service.Note("Scaled postgres pool 50->100", "")
	if err != nil {
		t.Fatalf("note failed: %v", err)
	}
	incident, _, err = service.Acknowledge("")
	if err != nil {
		t.Fatalf("ack failed: %v", err)
	}
	if incident.State != StateAcknowledged {
		t.Fatalf("expected state acknowledged, got %s", incident.State)
	}
	incident, _, err = service.Resolve("DB pool exhaustion", "")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if incident.State != StateResolved {
		t.Fatalf("expected state resolved, got %s", incident.State)
	}
	if len(incident.Events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(incident.Events))
	}
	for i := 1; i < len(incident.Events); i++ {
		if incident.Events[i].Timestamp.Before(incident.Events[i-1].Timestamp) {
			t.Fatalf("events out of order")
		}
	}
}

func TestServiceSingleActive(t *testing.T) {
	service := newTestService(t)
	if _, _, err := service.Start("order_api", P1, "Checkout timeouts", ""); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if _, _, err := service.Start("order_api", P1, "Second incident", ""); err == nil {
		t.Fatalf("expected error for duplicate active incident")
	}
}

func TestServiceResolveRequiresSummary(t *testing.T) {
	service := newTestService(t)
	if _, _, err := service.Start("order_api", P1, "Checkout timeouts", ""); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if _, _, err := service.Resolve("", ""); err == nil {
		t.Fatalf("expected error for empty summary")
	}
}
