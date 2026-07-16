package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

// Google OAuth credentials — same as the gog CLI credentials-niche.json
const gogClientID = "1019445936462-9vcsjgiq2d6qumb2ppjdqih00m5tlc5a.apps.googleusercontent.com"
// gogClientSecret is loaded from env at runtime

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

// gogTokenResponse is the Google OAuth2 token endpoint response.
type gogTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// gogExchangeCode exchanges an OAuth authorization code for tokens by posting
// directly to Google's token endpoint. This avoids the gog CLI step 2 subprocess
// which was unreliable (invalid_grant when code expired or was already used).
func gogExchangeCode(ctx context.Context, code string) (*gogTokenResponse, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", gogClientID)
	form.Set("client_secret", os.Getenv("CLICKCLACK_GOG_CLIENT_SECRET"))
	form.Set("redirect_uri", gogRedirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posting to token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tok gogTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.Error != "" {
		return nil, fmt.Errorf("token exchange error %s: %s", tok.Error, tok.ErrorDesc)
	}
	// refresh_token may be absent if user already consented (prompt=none flow)
	// We'll store whatever token we get; gog import needs refresh_token to work long-term
	// but we can still complete the flow
	return &tok, nil
}

// gogStoreRefreshToken stores a refresh token in the gog keyring using
// `gog auth import --refresh-token-stdin`. This is more reliable than
// invoking step 2 because we already have the token in hand.
func gogStoreRefreshToken(email, refreshToken string) error {
	cmd := exec.Command("/usr/local/bin/gog", "auth", "import",
		"--email", email,
		"--client", gogClient,
		"--services", gogServices,
		"--refresh-token-stdin",
		"--no-input",
	)
	cmd.Env = append(gogEnv(), "PATH=/usr/local/bin:/usr/bin:/bin")
	cmd.Stdin = strings.NewReader(refreshToken)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gog auth import failed: %w: %s", err, string(out))
	}
	return nil
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
	cmd := exec.CommandContext(context.Background(),
		"/usr/local/bin/gog", "auth", "add", email,
		"--client", gogClient,
		"--remote",
		"--step", "1",
		"--redirect-uri", gogRedirectURI,
		"--services", gogServices,
		"--force-consent",
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

	// Load persisted states (survives server restarts)
	gogStateLoad()
	// Prune stale entries opportunistically
	gogPruneExpired()

	// Store state -> email mapping with TTL
	gogStateSave(state, gogStateEntry{
		email:     email,
		expiresAt: time.Now().Add(gogStateTTL),
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// gogCallback receives the Google OAuth callback (no auth required - Google
// sends the browser here). It exchanges the auth code directly with Google's
// token endpoint, then stores the refresh token via `gog auth import`.
//
// GET /api/auth/gog/callback?code=...&state=...
func (s *Server) gogCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Redirect(w, r, "/api/auth/gog/start?error=missing_state", http.StatusFound)
		return
	}

	// Check for OAuth errors from Google
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		errDesc := r.URL.Query().Get("error_description")
		fmt.Printf("gog OAuth error from Google: %s: %s\n", oauthErr, errDesc)
		http.Redirect(w, r, "/api/auth/gog/start?error="+url.QueryEscape(oauthErr), http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/api/auth/gog/start?error=missing_code", http.StatusFound)
		return
	}

	// Look up email from state map (load persisted states first in case of restart)
	gogStateLoad()
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

	// Exchange the authorization code directly with Google's token endpoint.
	// We do this in Go rather than calling `gog auth add --step 2` because the
	// gog CLI subprocess was consistently returning invalid_grant — the code was
	// being treated as already-used or the round-trip was too slow.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := gogExchangeCode(ctx, code)
	if err != nil {
		fmt.Printf("gog token exchange failed for %s: %v\n", email, err)
		http.Error(w, "Google authentication failed (token exchange). Please close this tab and try again from Niche OS.", http.StatusBadGateway)
		return
	}

	// Store the refresh token in gog's keyring via `gog auth import`.
	if err := gogStoreRefreshToken(email, tok.RefreshToken); err != nil {
		fmt.Printf("gog auth import failed for %s: %v\n", email, err)
		http.Error(w, "Google authentication failed (token storage). Please close this tab and try again from Niche OS.", http.StatusInternalServerError)
		return
	}

	fmt.Printf("gog auth success for %s\n", email)
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
var gogStateFile = "/tmp/clickclack-gog-states.json"

func gogStateSave(state string, entry gogStateEntry) {
	gogStateMap.Store(state, entry)
	data := map[string]interface{}{}
	gogStateMap.Range(func(k, v any) bool {
		e := v.(gogStateEntry)
		if time.Now().Before(e.expiresAt) {
			data[k.(string)] = map[string]interface{}{"email": e.email, "expires_at": e.expiresAt.Unix()}
		}
		return true
	})
	if b, err := json.Marshal(data); err == nil {
		os.WriteFile(gogStateFile, b, 0600)
	}
}

func gogStateLoad() {
	b, err := os.ReadFile(gogStateFile)
	if err != nil { return }
	var data map[string]map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil { return }
	for state, v := range data {
		expiresUnix, _ := v["expires_at"].(float64)
		expiresAt := time.Unix(int64(expiresUnix), 0)
		if time.Now().Before(expiresAt) {
			email, _ := v["email"].(string)
			gogStateMap.Store(state, gogStateEntry{email: email, expiresAt: expiresAt})
		}
	}
}

func gogPruneExpired() {
	now := time.Now()
	gogStateMap.Range(func(k, v any) bool {
		if entry, ok := v.(gogStateEntry); ok && now.After(entry.expiresAt) {
			gogStateMap.Delete(k)
		}
		return true
	})
}
