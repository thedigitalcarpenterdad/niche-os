package httpapi

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// gogStateEntry holds the email associated with a gog OAuth state token, plus
// an expiry so we can clean up old entries without a background goroutine.
type gogStateEntry struct {
	email     string
	expiresAt time.Time
}

// gogStateMap is the server-wide store of pending gog OAuth states.
// Key: state string (from gog step 1 output)
// Value: gogStateEntry
var gogStateMap sync.Map

const gogStateTTL = 10 * time.Minute
const gogRedirectURI = "https://os.nichewaterproofing.com/api/auth/gog/callback"
const gogServices = "gmail,calendar,drive,docs,sheets,contacts"
const gogClient = "niche"

// gogHasToken checks whether the gog keyring has a stored token for the given email.
func gogHasToken(email string) bool {
	cmd := exec.Command("/usr/local/bin/gog", "auth", "list", "--plain")
	cmd.Env = append(gogEnv(), "PATH=/usr/local/bin:/usr/bin:/bin")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) > 0 && strings.EqualFold(strings.TrimSpace(fields[0]), strings.TrimSpace(email)) {
			return true
		}
	}
	return false
}

// gogEnv returns the environment variables required for gog CLI calls.
func gogEnv() []string {
	return []string{
		"GOG_KEYRING_PASSWORD=",
		"HOME=/root",
		"XDG_DATA_HOME=/root/.local/share",
	}
}

// gogStart initiates the gog OAuth flow for the currently-authenticated user.
// It runs gog step 1, stores the state, and redirects the browser to Google.
//
// GET /api/auth/gog/start
func (s *Server) gogStart(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	identity, err := s.store.GetIdentityByUserProvider(r.Context(), act.user.ID, "oidc")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no linked OIDC identity: %w", err))
		return
	}
	email := strings.TrimSpace(identity.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, errors.New("OIDC identity has no email"))
		return
	}

	// Run gog auth add step 1
	cmd := exec.CommandContext(r.Context(),
		"/usr/local/bin/gog", "auth", "add", email,
		"--client", gogClient,
		"--remote",
		"--step", "1",
		"--redirect-uri", gogRedirectURI,
		"--services", gogServices,
		"--no-input",
	)
	cmd.Env = append(gogEnv(), "PATH=/usr/local/bin:/usr/bin:/bin")

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		detail := ""
		if errors.As(err, &exitErr) {
			detail = string(exitErr.Stderr)
		}
		writeError(w, http.StatusInternalServerError, fmt.Errorf("gog step 1 failed: %w: %s", err, detail))
		return
	}

	// Parse tab-separated key=value pairs from stdout
	var authURL string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "auth_url":
			authURL = val
		}
	}

	if authURL == "" {
		writeError(w, http.StatusInternalServerError, errors.New("gog step 1 did not return auth_url"))
		return
	}

	// Extract state from auth_url (gog embeds it as a query param, not a separate output line)
	parsedURL, parseErr := url.Parse(authURL)
	if parseErr != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("unparseable auth_url: %w", parseErr))
		return
	}
	state := parsedURL.Query().Get("state")
	if state == "" {
		writeError(w, http.StatusInternalServerError, errors.New("gog auth_url missing state param"))
		return
	}

	// Prune stale entries opportunistically
	gogPruneExpired()

	// Store state -> email mapping with TTL
	gogStateMap.Store(state, gogStateEntry{
		email:     email,
		expiresAt: time.Now().Add(gogStateTTL),
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// gogCallback receives the Google OAuth callback (no auth required — Google
// sends the browser here).
//
// GET /api/auth/gog/callback?code=...&state=...
func (s *Server) gogCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Redirect(w, r, "/api/auth/gog/start?error=missing_state", http.StatusFound)
		return
	}

	// Look up email from state map
	raw, ok := gogStateMap.LoadAndDelete(state)
	if !ok {
		http.Redirect(w, r, "/api/auth/gog/start?error=unknown_state", http.StatusFound)
		return
	}
	entry, ok := raw.(gogStateEntry)
	if !ok || time.Now().After(entry.expiresAt) {
		http.Redirect(w, r, "/api/auth/gog/start?error=expired_state", http.StatusFound)
		return
	}
	email := entry.email

	// Reconstruct the full callback URL
	fullCallbackURL := gogRedirectURI + "?" + r.URL.RawQuery

	// Run gog auth add step 2
	cmd := exec.CommandContext(r.Context(),
		"/usr/local/bin/gog", "auth", "add", email,
		"--client", gogClient,
		"--remote",
		"--step", "2",
		"--redirect-uri", gogRedirectURI,
		"--services", gogServices,
		"--auth-url", fullCallbackURL,
	)
	cmd.Env = append(gogEnv(), "PATH=/usr/local/bin:/usr/bin:/bin")

	if out, err := cmd.CombinedOutput(); err != nil {
		// Log the failure but don't expose internal detail to the browser
		fmt.Printf("gog step 2 failed for %s: %v\noutput: %s\n", email, err, string(out))
		http.Redirect(w, r, "/api/auth/gog/start?error=1", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// gogStatus returns whether the currently-authenticated user has a gog token.
//
// GET /api/auth/gog/status
func (s *Server) gogStatus(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	identity, err := s.store.GetIdentityByUserProvider(r.Context(), act.user.ID, "oidc")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no linked OIDC identity: %w", err))
		return
	}
	email := strings.TrimSpace(identity.Email)

	writeJSON(w, http.StatusOK, map[string]any{
		"has_token": gogHasToken(email),
		"email":     email,
	})
}

// gogPruneExpired removes expired state entries from gogStateMap.
func gogPruneExpired() {
	now := time.Now()
	gogStateMap.Range(func(k, v any) bool {
		if entry, ok := v.(gogStateEntry); ok && now.After(entry.expiresAt) {
			gogStateMap.Delete(k)
		}
		return true
	})
}
