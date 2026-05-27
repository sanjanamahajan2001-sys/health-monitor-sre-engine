package incident

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestStoreSaveLoadList(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)
	now := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	incident := Incident{
		ID:        "INC-20260204-172300",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateStarted,
		CreatedAt: now,
		UpdatedAt: now,
		Events: []Event{
			{Timestamp: now, User: "sanjana", Type: EventStart},
		},
	}
	if err := store.Save(incident); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := store.Load(incident.ID)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.ID != incident.ID || loaded.Service != incident.Service {
		t.Fatalf("loaded incident mismatch")
	}
	list, warnings, err := store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(list))
	}
}

func TestStoreListSkipsCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreWithDir(dir)
	if err := os.WriteFile(filepath.Join(dir, "INC-20260204-172300.json"), []byte("{invalid"), 0600); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	list, warnings, err := store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no incidents, got %d", len(list))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings for corrupt file")
	}
}

func TestStoreRespectsCustomPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEALTH_MONITOR_INCIDENTS_PATH", dir)
	store, err := NewStore()
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	now := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	incident := Incident{
		ID:        "INC-20260204-172300",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateStarted,
		CreatedAt: now,
		UpdatedAt: now,
		Events: []Event{
			{Timestamp: now, User: "sanjana", Type: EventStart},
		},
	}
	if err := store.Save(incident); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, incident.ID+".json")); err != nil {
		t.Fatalf("expected incident file in custom dir: %v", err)
	}
}

func TestStoreAppliesSudoOwnership(t *testing.T) {
	current, err := user.Current()
	if err != nil || current.Username == "" {
		t.Skip("unable to determine current user")
	}
	dir := t.TempDir()
	t.Setenv("HEALTH_MONITOR_INCIDENTS_PATH", dir)
	t.Setenv("SUDO_USER", current.Username)
	store, err := NewStore()
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	now := time.Date(2026, 2, 4, 17, 23, 0, 0, time.UTC)
	incident := Incident{
		ID:        "INC-20260204-172300",
		Service:   "order_api",
		Severity:  P1,
		Title:     "Checkout timeouts",
		State:     StateStarted,
		CreatedAt: now,
		UpdatedAt: now,
		Events: []Event{
			{Timestamp: now, User: "sanjana", Type: EventStart},
		},
	}
	if err := store.Save(incident); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, incident.ID+".json"))
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("unable to read file ownership")
	}
	uid, _ := strconv.Atoi(current.Uid)
	if uid != int(stat.Uid) {
		t.Fatalf("expected uid %d, got %d", uid, stat.Uid)
	}
}
