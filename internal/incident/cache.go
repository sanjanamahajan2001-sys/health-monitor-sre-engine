package incident

import "sync"

var activeOnce sync.Once
var cachedActive []Incident
var cachedActiveWarnings []string
var cachedActiveErr error

func ListActiveCached() ([]Incident, []string, error) {
	activeOnce.Do(func() {
		service, err := NewService(nil)
		if err != nil {
			cachedActiveErr = err
			return
		}
		cachedActive, cachedActiveWarnings, cachedActiveErr = service.ListActive()
	})
	return cachedActive, cachedActiveWarnings, cachedActiveErr
}

func ResetActiveCacheForTest() {
	activeOnce = sync.Once{}
	cachedActive = nil
	cachedActiveWarnings = nil
	cachedActiveErr = nil
}
