package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// SecretProvider defines the interface for retrieving secrets
type SecretProvider interface {
	// GetSecret retrieves a secret by its key or identifier
	GetSecret(key string) (string, error)
}

// FileSecretProvider retrieves secrets from local files
type FileSecretProvider struct {
	// Root directories to search for secrets (e.g., /etc/health-monitor, ~/.health-monitor)
	SearchPaths []string
}

// NewFileSecretProvider creates a provider with default search paths based on sudo context
func NewFileSecretProvider() *FileSecretProvider {
	paths := []string{systemConfigPath}
	
	// Add user home directory if not running exclusively as system root
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".health-monitor"))
	}
	
	// If running as sudo, also check the original user's home
	if os.Geteuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
				paths = append(paths, filepath.Join(u.HomeDir, ".health-monitor"))
			}
		}
	}
	
	return &FileSecretProvider{SearchPaths: paths}
}

func (p *FileSecretProvider) GetSecret(filename string) (string, error) {
	for _, dir := range p.SearchPaths {
		path := filepath.Join(dir, filename)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			data, err := os.ReadFile(path)
			if err == nil {
				return strings.TrimSpace(string(data)), nil
			}
		}
	}
	return "", fmt.Errorf("secret file %q not found in search paths", filename)
}

// EnvSecretProvider retrieves secrets from environment variables
type EnvSecretProvider struct {
	// Optional prefix for environment variables (e.g., HM_SECRET_)
	Prefix string
}

func (p *EnvSecretProvider) GetSecret(key string) (string, error) {
	envKey := p.Prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if val := os.Getenv(envKey); val != "" {
		return val, nil
	}
	return "", fmt.Errorf("environment variable %q not set", envKey)
}

// MultiSecretProvider tries multiple providers in order
type MultiSecretProvider struct {
	Providers []SecretProvider
}

func (p *MultiSecretProvider) GetSecret(key string) (string, error) {
	for _, provider := range p.Providers {
		if val, err := provider.GetSecret(key); err == nil && val != "" {
			return val, nil
		}
	}
	return "", fmt.Errorf("secret %q not found in any provider", key)
}

// GetGlobalSecretProvider returns a default multi-provider (Env then File)
func GetGlobalSecretProvider() SecretProvider {
	return &MultiSecretProvider{
		Providers: []SecretProvider{
			&EnvSecretProvider{Prefix: "HM_SECRET_"},
			NewFileSecretProvider(),
		},
	}
}
