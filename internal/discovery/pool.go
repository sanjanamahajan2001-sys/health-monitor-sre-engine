package discovery

import (
	"sync"
)

// ServicePool manages a pool of workers to parallelize service-level operations
type ServicePool struct {
	workerCount int
}

// NewServicePool creates a new ServicePool with the specified number of workers
func NewServicePool(workers int) *ServicePool {
	if workers <= 0 {
		workers = 10 // Default
	}
	return &ServicePool{
		workerCount: workers,
	}
}

// Result represents the outcome of a single probe
type Result struct {
	Service string
	Error   error
	Data    interface{}
}

// ParallelProbe runs the provided probe function across all services in parallel using the worker pool
func (p *ServicePool) ParallelProbe(services []string, probeFunc func(string) (interface{}, error)) []Result {
	if len(services) == 0 {
		return nil
	}

	workerCount := p.workerCount
	if len(services) < workerCount {
		workerCount = len(services)
	}

	svcChan := make(chan string, len(services))
	resChan := make(chan Result, len(services))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for svc := range svcChan {
				data, err := probeFunc(svc)
				resChan <- Result{
					Service: svc,
					Error:   err,
					Data:    data,
				}
			}
		}()
	}

	// Feed services to workers
	for _, svc := range services {
		svcChan <- svc
	}
	close(svcChan)

	// Wait for workers in a separate goroutine
	go func() {
		wg.Wait()
		close(resChan)
	}()

	results := make([]Result, 0, len(services))
	for res := range resChan {
		results = append(results, res)
	}

	return results
}
