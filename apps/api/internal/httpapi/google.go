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

type GoogleOAuthConfig struct {
	ClientID      string
	ClientSecret  string
	AllowedDomain string // e.g. "nichewaterproofing.com"
	PublicURL     string
	AuthURL       string
	TokenURL      string
	UserInfoURL   string
	HTTPClient    *http.Client
}

var (
	errGoogleDomainDenied = errors.New("google account is not in the allowed domain")
	errGoogleNotConfigured = errors.New("google oauth is not configured")
)

const defaultGoogleHTTPTimeout = 30 * time.Second

func (c GoogleOAuthConfig) withDefaults() GoogleOAuthConfig {
	if c.AuthURL == "" {
		c.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if c.UserInfoURL == "" {
		c.UserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultGoogleHTTPTimeout}
	}
	return c
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	if s.googleOAuth.ClientID == "" || s.googleOAuth.ClientSecret == "" {
		writeError(w, http.StatusNotImplemented, errGoogleNotConfigured)
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "cc_google_state", Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: s.secureCookies(r), SameSite: http.SameSiteLaxMode})
	values := url.Values{
		"client_id":     {s.googleOAuth.ClientID},
		"redirect_uri":  {s.googleRedirectURL(r)},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"access_type":   {"online"},
		"prompt":        {"select_account"},
	}
	if domain := strings.TrimSpace(s.googleOAuth.AllowedDomain); domain != "" {
		values.Set("hd", domain)
	}
	http.Redirect(w, r, s.googleOAuth.AuthURL+"?"+values.Encode(), http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("cc_google_state")
	if err != nil || state.Value == "" || state.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, errors.New("invalid google oauth state"))
		return
	}
	s.clearGoogleStateCookie(w, r)
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, errors.New("google oauth code is required"))
		return
	}
	token, err := s.exchangeGoogleCode(r.Context(), r, code)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	profile, err := s.fetchGoogleProfile(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.ensureGoogleAllowedDomain(profile); err != nil {
		if errors.Is(err, errGoogleDomainDenied) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	user, err := s.store.UpsertIdentityUser(r.Context(), store.UpsertIdentityUserInput{
		Provider:        "google",
		ProviderSubject: profile.Sub,
		Email:           profile.Email,
		DisplayName:     firstNonEmpty(profile.Name, profile.Email),
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

func (s *Server) clearGoogleStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cc_google_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) exchangeGoogleCode(ctx context.Context, r *http.Request, code string) (string, error) {
	body := url.Values{
		"client_id":     {s.googleOAuth.ClientID},
		"client_secret": {s.googleOAuth.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.googleRedirectURL(r)},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.googleOAuth.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.googleOAuth.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", errors.New("google token exchange failed")
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
		return "", errors.New("google access token missing")
	}
	return out.AccessToken, nil
}

type googleProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	HostedDomain  string `json:"hd"`
}

func (s *Server) fetchGoogleProfile(ctx context.Context, token string) (googleProfile, error) {
	var profile googleProfile
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.googleOAuth.UserInfoURL, nil)
	if err != nil {
		return googleProfile{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.googleOAuth.HTTPClient.Do(req)
	if err != nil {
		return googleProfile{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return googleProfile{}, errors.New("google userinfo request failed")
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return googleProfile{}, err
	}
	if profile.Sub == "" {
		return googleProfile{}, errors.New("google profile subject missing")
	}
	if profile.Email == "" {
		return googleProfile{}, errors.New("google profile email missing")
	}
	return profile, nil
}

func (s *Server) ensureGoogleAllowedDomain(profile googleProfile) error {
	domain := strings.TrimSpace(strings.ToLower(s.googleOAuth.AllowedDomain))
	if domain == "" {
		return nil
	}
	// Prefer the verified hosted-domain claim; fall back to the email suffix.
	if strings.EqualFold(strings.TrimSpace(profile.HostedDomain), domain) {
		return nil
	}
	email := strings.ToLower(strings.TrimSpace(profile.Email))
	if strings.HasSuffix(email, "@"+domain) {
		return nil
	}
	return errGoogleDomainDenied
}

func (s *Server) googleRedirectURL(r *http.Request) string {
	base := strings.TrimRight(s.googleOAuth.PublicURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/api/auth/google/callback"
}
