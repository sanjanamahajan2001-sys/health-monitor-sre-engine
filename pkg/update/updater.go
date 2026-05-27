package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"health-monitor/pkg/model"
)

const (
	// DefaultUpdateBaseURL is the base URL for downloading updates
	// Can be overridden via UPDATE_BASE_URL environment variable
	DefaultUpdateBaseURL = "https://pub-0cf86c67f6dd456087607707a5a37f73.r2.dev"
	// EnvDisableUpdates disables update checks when set to a truthy value.
	EnvDisableUpdates = "HEALTH_MONITOR_DISABLE_UPDATES"
	// UpdateCheckTimeout is the maximum time to wait for update check
	UpdateCheckTimeout = 5 * time.Second
	// UpdateDownloadTimeout is the maximum time to wait for binary download
	UpdateDownloadTimeout = 30 * time.Second
	// EnvAllowInsecureUpdates allows non-https update URLs when set.
	EnvAllowInsecureUpdates = "HEALTH_MONITOR_ALLOW_INSECURE_UPDATES"
)

// UpdateResult represents the result of an update check/operation
type UpdateResult struct {
	UpdateAvailable bool
	CurrentVersion  string
	LatestVersion   string
	Updated         bool
	Message         string
}

func UpdatesDisabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(EnvDisableUpdates)))
	return value != "" && value != "0" && value != "false" && value != "no"
}

// CheckForUpdate checks if a newer version is available
func CheckForUpdate() (*UpdateResult, error) {
	result := &UpdateResult{
		CurrentVersion: model.Version,
	}

	// Get latest version from remote
	latestVersion, err := fetchLatestVersion()
	if err != nil {
		// Return error so caller can log it
		return result, fmt.Errorf("version check failed: %w", err)
	}

	result.LatestVersion = latestVersion

	// Compare versions
	if compareVersions(model.Version, latestVersion) < 0 {
		result.UpdateAvailable = true
	}

	return result, nil
}

// PerformUpdate downloads and installs the latest version
func PerformUpdate() (*UpdateResult, error) {
	result := &UpdateResult{
		CurrentVersion: model.Version,
	}

	// Get latest version
	latestVersion, err := fetchLatestVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest version: %w", err)
	}

	result.LatestVersion = latestVersion

	// Check if update is needed
	if compareVersions(model.Version, latestVersion) >= 0 {
		result.Message = fmt.Sprintf("Already running latest version %s", model.Version)
		return result, nil
	}

	// Get current binary path
	currentPath, err := getCurrentBinaryPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get current binary path: %w", err)
	}

	// Download new binary
	baseURL := getUpdateBaseURL()
	// Remove trailing slash if present
	baseURL = strings.TrimSuffix(baseURL, "/")
	binaryURL := fmt.Sprintf("%s/health-monitor-linux-amd64-%s", baseURL, latestVersion)

	tempPath, err := downloadBinary(binaryURL, latestVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to download binary: %w", err)
	}
	defer os.Remove(tempPath) // Clean up temp file

	// Verify checksum if available
	if err := verifyChecksum(binaryURL, tempPath); err != nil {
		return nil, fmt.Errorf("checksum verification failed: %w", err)
	}

	// Verify downloaded binary
	if err := verifyBinary(tempPath); err != nil {
		return nil, fmt.Errorf("binary verification failed: %w", err)
	}

	// Replace current binary atomically
	if err := replaceBinary(currentPath, tempPath); err != nil {
		return nil, fmt.Errorf("failed to replace binary: %w", err)
	}

	// Note: We don't create a flag file anymore since we show the message
	// immediately after update and the next run will use the new binary silently

	result.Updated = true
	result.UpdateAvailable = false
	result.Message = fmt.Sprintf("Successfully updated to version %s", latestVersion)

	return result, nil
}

// CheckAndPerformUpdate checks for updates and performs them if available
// This function has a timeout to prevent blocking execution for too long
func CheckAndPerformUpdate() (*UpdateResult, error) {
	// Use a channel with timeout to prevent blocking
	resultChan := make(chan *UpdateResult, 1)
	errChan := make(chan error, 1)

	go func() {
		checkResult, err := CheckForUpdate()
		if err != nil {
			errChan <- err
			return
		}

		if !checkResult.UpdateAvailable {
			resultChan <- checkResult
			return
		}

		updateResult, err := PerformUpdate()
		if err != nil {
			errChan <- err
			return
		}

		resultChan <- updateResult
	}()

	// Wait for result with overall timeout
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(UpdateCheckTimeout + UpdateDownloadTimeout + 5*time.Second):
		// Overall timeout - return nil result (update check failed silently)
		return &UpdateResult{
			CurrentVersion: model.Version,
			Message:        "Update check timed out",
		}, nil
	}
}

// fetchLatestVersion fetches the latest version from remote
func fetchLatestVersion() (string, error) {
	baseURL := getUpdateBaseURL()
	// Remove trailing slash if present
	baseURL = strings.TrimSuffix(baseURL, "/")
	url := fmt.Sprintf("%s/version.txt", baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), UpdateCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	client := &http.Client{
		Timeout: UpdateCheckTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(string(body))
	if version == "" {
		return "", fmt.Errorf("empty version string")
	}

	// Ensure version starts with 'v'
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	return version, nil
}

// downloadBinary downloads the binary from the given URL
func downloadBinary(url, version string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), UpdateDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: UpdateDownloadTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download: status code %d", resp.StatusCode)
	}

	// Create temp file
	tempFile, err := os.CreateTemp("", fmt.Sprintf("health-monitor-%s-*.tmp", version))
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()

	// Download to temp file
	_, err = io.Copy(tempFile, resp.Body)
	tempFile.Close()

	if err != nil {
		os.Remove(tempPath)
		return "", err
	}

	// Make executable
	if err := os.Chmod(tempPath, 0755); err != nil {
		os.Remove(tempPath)
		return "", err
	}

	return tempPath, nil
}

// verifyBinary verifies the downloaded binary is valid
func verifyBinary(path string) error {
	// Check if file exists and is executable
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.Size() == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	// Try to execute it with --version flag to verify it's a valid binary
	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	if err := cmd.Run(); err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("binary verification timed out")
		}
		return fmt.Errorf("binary verification failed: %w", err)
	}

	return nil
}

func verifyChecksum(binaryURL string, path string) error {
	checksumURL := binaryURL + ".sha256"
	client := http.Client{Timeout: UpdateDownloadTimeout}
	resp, err := client.Get(checksumURL)
	if err != nil {
		// If checksum isn't available, skip verification to avoid breaking updates.
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return fmt.Errorf("empty checksum response")
	}
	expected := strings.TrimSpace(fields[0])
	if _, err := hex.DecodeString(expected); err != nil {
		return fmt.Errorf("invalid checksum format")
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}

// replaceBinary atomically replaces the current binary with the new one
func replaceBinary(currentPath, newPath string) error {
	// Get directory and filename
	dir := filepath.Dir(currentPath)
	filename := filepath.Base(currentPath)

	// Create a backup path
	backupPath := filepath.Join(dir, filename+".backup")

	// Copy current binary to backup (for rollback if needed)
	if err := copyFile(currentPath, backupPath); err != nil {
		// Non-fatal, continue anyway - backup is just for safety
		fmt.Fprintf(os.Stderr, "Warning: failed to create backup: %v\n", err)
	}

	// On Unix/Linux, we can replace a running binary, but the current process
	// will continue using the old in-memory copy. The new binary will be used
	// on the next execution. This is safe and standard practice.

	// Strategy: Use atomic rename operation
	// 1. Copy new binary to a temp file in the same directory
	tempTarget := filepath.Join(dir, filename+".new")

	// Copy new binary to temp location
	if err := copyFile(newPath, tempTarget); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("failed to copy new binary: %w", err)
	}

	// Make sure it's executable
	if err := os.Chmod(tempTarget, 0755); err != nil {
		os.Remove(tempTarget)
		os.Remove(backupPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename: this is the key operation
	// On most Unix filesystems, rename is atomic
	if err := os.Rename(tempTarget, currentPath); err != nil {
		os.Remove(tempTarget)
		// Try to restore backup if rename failed
		if backupErr := copyFile(backupPath, currentPath); backupErr == nil {
			os.Remove(backupPath)
		}
		return fmt.Errorf("failed to replace binary (rename failed): %w", err)
	}

	// Clean up backup after successful replacement
	os.Remove(backupPath)

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// getCurrentBinaryPath gets the path to the currently running binary
func getCurrentBinaryPath() (string, error) {
	// Get the path of the current executable
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Resolve symlinks to get actual path
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		// If symlink resolution fails, use the original path
		realPath = exePath
	}

	return realPath, nil
}

// getUpdateBaseURL returns the base URL for updates
func getUpdateBaseURL() string {
	if url := os.Getenv("UPDATE_BASE_URL"); url != "" {
		trimmed := strings.TrimSpace(url)
		parsed, err := urlpkg.Parse(trimmed)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return DefaultUpdateBaseURL
		}
		if !strings.EqualFold(parsed.Scheme, "https") && !allowInsecureUpdates() {
			return DefaultUpdateBaseURL
		}
		return strings.TrimSuffix(trimmed, "/")
	}
	return DefaultUpdateBaseURL
}

func allowInsecureUpdates() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(EnvAllowInsecureUpdates)))
	return value != "" && value != "0" && value != "false" && value != "no"
}

// compareVersions compares two version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Split by dots
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &p1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &p2)
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}

	return 0
}

// GetPlatformSuffix returns the platform suffix for the current system
func GetPlatformSuffix() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	return fmt.Sprintf("%s-%s", goos, goarch)
}
