package model

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultVersionURL is the default endpoint to fetch the latest version
	// This can be overridden via VERSION_URL environment variable
	DefaultVersionURL = "https://releases.sofueled.com/health-monitor/version.txt"
	// VersionFetchTimeout is the maximum time to wait for version fetch
	VersionFetchTimeout = 2 * time.Second
)

var (
	// cachedLatestVersion stores the fetched version to avoid repeated requests
	cachedLatestVersion string
	// versionFetchAttempted tracks if we've already tried to fetch
	versionFetchAttempted bool
)

// GetLatestVersion attempts to fetch the latest version from remote source.
// Returns the fetched version if successful, otherwise returns the current Version.
// This function is safe to call multiple times and will cache the result.
func GetLatestVersion() string {
	// Return cached version if available
	if cachedLatestVersion != "" {
		return cachedLatestVersion
	}

	// If we've already attempted and failed, return current version
	if versionFetchAttempted {
		return Version
	}

	// Mark as attempted
	versionFetchAttempted = true

	// Try to fetch in background (non-blocking)
	// Use a channel to get result with timeout
	resultChan := make(chan string, 1)
	go func() {
		version := fetchVersionFromRemote()
		if version != "" {
			resultChan <- version
		} else {
			resultChan <- Version // fallback to current
		}
	}()

	// Wait for result with timeout
	select {
	case fetchedVersion := <-resultChan:
		if fetchedVersion != Version {
			cachedLatestVersion = fetchedVersion
			return fetchedVersion
		}
		return Version
	case <-time.After(VersionFetchTimeout):
		// Timeout - return current version
		return Version
	}
}

// fetchVersionFromRemote attempts to fetch version from remote URL
func fetchVersionFromRemote() string {
	url := DefaultVersionURL
	
	// Check for environment variable override
	if envURL := os.Getenv("VERSION_URL"); envURL != "" {
		url = envURL
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), VersionFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	client := &http.Client{
		Timeout: VersionFetchTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// Clean up the version string (remove whitespace, ensure v prefix)
	version := strings.TrimSpace(string(body))
	if version == "" {
		return ""
	}

	// Ensure version starts with 'v' if it doesn't already
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return version
}

// ResetVersionCache clears the cached version (useful for testing)
func ResetVersionCache() {
	cachedLatestVersion = ""
	versionFetchAttempted = false
}
