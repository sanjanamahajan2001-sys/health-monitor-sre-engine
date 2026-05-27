package discovery

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestServicePool_ParallelProbe(t *testing.T) {
	pool := NewServicePool(5)
	services := []string{"svc1", "svc2", "svc3", "svc4", "svc5", "svc6", "svc7", "svc8", "svc9", "svc10"}

	var callCount int32
	probeFunc := func(svc string) (interface{}, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(10 * time.Millisecond) // Simulate work
		if svc == "svc3" {
			return nil, errors.New("error at svc3")
		}
		return svc + "-data", nil
	}

	start := time.Now()
	results := pool.ParallelProbe(services, probeFunc)
	duration := time.Since(start)

	if int(callCount) != len(services) {
		t.Errorf("Expected %d calls, got %d", len(services), callCount)
	}

	if len(results) != len(services) {
		t.Errorf("Expected %d results, got %d", len(services), len(results))
	}

	// Since we have 10 services and 5 workers, it should take at least 2 * 10ms = 20ms
	// but significantly less than 10 * 10ms = 100ms
	if duration >= 50*time.Millisecond {
		t.Errorf("Parallel execution took too long: %v", duration)
	}

	foundError := false
	for _, res := range results {
		if res.Service == "svc3" {
			if res.Error == nil || res.Error.Error() != "error at svc3" {
				t.Errorf("Expected error for svc3, got %v", res.Error)
			}
			foundError = true
		} else {
			if res.Error != nil {
				t.Errorf("Unexpected error for %s: %v", res.Service, res.Error)
			}
			if res.Data != res.Service+"-data" {
				t.Errorf("Unexpected data for %s: %v", res.Service, res.Data)
			}
		}
	}

	if !foundError {
		t.Error("svc3 error result not found")
	}
}
