package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

// OIDCConfig configures a generic OpenID Connect identity provider.
// At Niche this points at the self-hosted Logto instance
// (https://auth.nichewaterproofing.com/oidc).
type OIDCConfig struct {
	ClientID     string
	ClientSecret string
	Issuer       string // e.g. https://auth.nichewaterproofing.com/oidc
	PublicURL    string
	Scope        string
	// Endpoints are derived from Issuer when empty.
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	HTTPClient  *http.Client
}

var errOIDCNotConfigured = errors.New("oidc is not configured")

const defaultOIDCHTTPTimeout = 30 * time.Second

func (c OIDCConfig) withDefaults() OIDCConfig {
	issuer := strings.TrimRight(strings.TrimSpace(c.Issuer), "/")
	if c.AuthURL == "" && issuer != "" {
		c.AuthURL = issuer + "/auth"
	}
	if c.TokenURL == "" && issuer != "" {
		c.TokenURL = issuer + "/token"
	}
	if c.UserInfoURL == "" && issuer != "" {
		c.UserInfoURL = issuer + "/me"
	}
	if c.Scope == "" {
		c.Scope = "openid email profile phone"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultOIDCHTTPTimeout}
	}
	return c
}

func (s *Server) oidcConfigured() bool {
	return s.oidc.ClientID != "" && s.oidc.ClientSecret != "" && s.oidc.AuthURL != "" && s.oidc.TokenURL != ""
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if !s.oidcConfigured() {
		writeError(w, http.StatusNotImplemented, errOIDCNotConfigured)
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "cc_oidc_state", Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: s.secureCookies(r), SameSite: http.SameSiteLaxMode})
	values := url.Values{
		"client_id":     {s.oidc.ClientID},
		"redirect_uri":  {s.oidcRedirectURL(r)},
		"response_type": {"code"},
		"scope":         {s.oidc.Scope},
		"state":         {state},
	}
	http.Redirect(w, r, s.oidc.AuthURL+"?"+values.Encode(), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("cc_oidc_state")
	if err != nil || state.Value == "" || state.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, errors.New("invalid oidc state"))
		return
	}
	s.clearOIDCStateCookie(w, r)
	if oidcErr := strings.TrimSpace(r.URL.Query().Get("error")); oidcErr != "" {
		writeError(w, http.StatusBadGateway, errors.New("oidc provider error: "+oidcErr))
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, errors.New("oidc code is required"))
		return
	}
	token, err := s.exchangeOIDCCode(r.Context(), r, code)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	profile, err := s.fetchOIDCProfile(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	display := firstNonEmpty(profile.Name, profile.Username, profile.Email, profile.PhoneNumber, profile.Sub)
	user, err := s.store.UpsertIdentityUser(r.Context(), store.UpsertIdentityUserInput{
		Provider:        "oidc",
		ProviderSubject: profile.Sub,
		Email:           profile.Email,
		DisplayName:     display,
		AvatarURL:       profile.Picture,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.store.EnsureDefaultWorkspaceMember(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setSessionCookie(w, r, session)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) clearOIDCStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cc_oidc_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) exchangeOIDCCode(ctx context.Context, r *http.Request, code string) (string, error) {
	body := url.Values{
		"client_id":     {s.oidc.ClientID},
		"client_secret": {s.oidc.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.oidcRedirectURL(r)},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.oidc.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.oidc.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", errors.New("oidc token exchange failed")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	if out.AccessToken == "" {
		return "", errors.New("oidc access token missing")
	}
	return out.AccessToken, nil
}

type oidcProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	Picture       string `json:"picture"`
	PhoneNumber   string `json:"phone_number"`
}

func (s *Server) fetchOIDCProfile(ctx context.Context, token string) (oidcProfile, error) {
	var profile oidcProfile
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.oidc.UserInfoURL, nil)
	if err != nil {
		return oidcProfile{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.oidc.HTTPClient.Do(req)
	if err != nil {
		return oidcProfile{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return oidcProfile{}, errors.New("oidc userinfo request failed")
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return oidcProfile{}, err
	}
	if profile.Sub == "" {
		return oidcProfile{}, errors.New("oidc profile subject missing")
	}
	return profile, nil
}

func (s *Server) oidcRedirectURL(r *http.Request) string {
	base := strings.TrimRight(s.oidc.PublicURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/api/auth/logto/callback"
}
