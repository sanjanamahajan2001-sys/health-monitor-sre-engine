package alert

import (
	"errors"
	"net/http"
	"os"
	"strings"
)

type AuthConfig struct {
	Token string
	User  string
	Pass  string
}

func LoadAuthConfig(token string, tokenFile string, user string, pass string, basicFile string) (AuthConfig, error) {
	cfg := AuthConfig{
		Token: strings.TrimSpace(token),
		User:  strings.TrimSpace(user),
		Pass:  strings.TrimSpace(pass),
	}
	if cfg.Token == "" && strings.TrimSpace(tokenFile) != "" {
		if data, err := os.ReadFile(strings.TrimSpace(tokenFile)); err == nil {
			cfg.Token = strings.TrimSpace(string(data))
		} else {
			return cfg, err
		}
	}
	if cfg.User == "" && cfg.Pass == "" && strings.TrimSpace(basicFile) != "" {
		data, err := os.ReadFile(strings.TrimSpace(basicFile))
		if err != nil {
			return cfg, err
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
		if len(parts) != 2 {
			return cfg, errors.New("invalid basic auth file format")
		}
		cfg.User = strings.TrimSpace(parts[0])
		cfg.Pass = strings.TrimSpace(parts[1])
	}
	return cfg, nil
}

func authMiddleware(next http.Handler, cfg AuthConfig) http.Handler {
	if strings.TrimSpace(cfg.Token) == "" && strings.TrimSpace(cfg.User) == "" && strings.TrimSpace(cfg.Pass) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedRequest(r, cfg) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="health-monitor"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func allowedRequest(r *http.Request, cfg AuthConfig) bool {
	if strings.TrimSpace(cfg.Token) != "" {
		if tokenFromHeader(r) == cfg.Token {
			return true
		}
	}
	if strings.TrimSpace(cfg.User) != "" || strings.TrimSpace(cfg.Pass) != "" {
		user, pass, ok := r.BasicAuth()
		if ok && user == cfg.User && pass == cfg.Pass {
			return true
		}
	}
	return false
}

func tokenFromHeader(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-Auth-Token")); token != "" {
		return token
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}
