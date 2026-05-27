package config

import (
	"log"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors configuration directories for changes and triggers reloads
type Watcher struct {
	watcher *fsnotify.Watcher
	mu      sync.Mutex
	paths   []string
	done    chan bool
}

// NewWatcher creates a new configuration watcher
func NewWatcher() (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher: fsWatcher,
		done:    make(chan bool),
	}, nil
}

// AddPath adds a directory or file to the watcher
func (w *Watcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if _, err := os.Stat(path); err != nil {
		return nil // Ignore non-existent paths (common for flows.d/alerts.d)
	}
	
	err := w.watcher.Add(path)
	if err == nil {
		w.paths = append(w.paths, path)
	}
	return err
}

// Start begins the watching loop. When a change is detected, reloadFunc is called.
func (w *Watcher) Start(reloadFunc func()) {
	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				// We care about writes, renames, and removals
				if event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Remove == fsnotify.Remove ||
					event.Op&fsnotify.Rename == fsnotify.Rename {
					log.Printf("INFO: Config change detected in %s (%s). Triggering hot-reload...", event.Name, event.Op)
					reloadFunc()
				}
			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("ERROR: Config watcher error: %v", err)
			case <-w.done:
				return
			}
		}
	}()
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.done)
	w.watcher.Close()
}

// SetupHotReload initializes a watcher for the active profile's config directories
func SetupHotReload(reloadFunc func()) *Watcher {
	pm := GetProfileManager()
	w, err := NewWatcher()
	if err != nil {
		log.Printf("ERROR: Failed to initialize config watcher: %v", err)
		return nil
	}

	// Watch profiles directory
	w.AddPath(pm.GetProfilesPath())
	
	// Watch flows.d and alerts.d
	w.AddPath(pm.GetFlowsPath())
	w.AddPath(pm.GetAlertsPath())

	w.Start(reloadFunc)
	return w
}
