package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

// TelegramAuthConfig holds Telegram Login Widget config.
type TelegramAuthConfig struct {
	BotToken    string
	BotUsername string // e.g. "Niche99Bot"
}

// telegramStore is the optional store capability required for Telegram auth.
// The sqlite store implements this; other stores may not.
type telegramStore interface {
	CheckTelegramWhitelist(ctx context.Context, telegramID string) (store.TelegramWhitelistEntry, error)
	UpsertTelegramUser(ctx context.Context, telegramID, username, displayName, role string) (store.User, error)
}

// verifyTelegramAuth verifies the Telegram Login Widget hash.
// See: https://core.telegram.org/widgets/login#checking-authorization
func verifyTelegramAuth(data map[string]string, botToken string) error {
	hash, ok := data["hash"]
	if !ok || hash == "" {
		return errors.New("missing hash")
	}

	// Check auth_date not too old (5 minutes).
	authDate, err := strconv.ParseInt(data["auth_date"], 10, 64)
	if err != nil || time.Now().Unix()-authDate > 300 {
		return errors.New("auth data expired")
	}

	// Build data check string: sorted key=value pairs except hash.
	var parts []string
	for k, v := range data {
		if k != "hash" {
			parts = append(parts, k+"="+v)
		}
	}
	sort.Strings(parts)
	checkString := strings.Join(parts, "\n")

	// Secret key = SHA256(bot_token).
	h := sha256.New()
	h.Write([]byte(botToken))
	secretKey := h.Sum(nil)

	// HMAC-SHA256 of checkString with secretKey.
	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(checkString))
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedHash), []byte(hash)) {
		return errors.New("invalid hash — auth data tampered")
	}
	return nil
}

// handleTelegramLogin reports whether Telegram auth is configured and returns
// the bot username used to render the login widget.
func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	if s.telegramAuth.BotToken == "" || s.telegramAuth.BotUsername == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"bot_username": s.telegramAuth.BotUsername,
	})
}

// handleTelegramCallback processes the Telegram Login Widget callback.
// GET /api/auth/telegram/callback?id=...&first_name=...&hash=...&auth_date=...
func (s *Server) handleTelegramCallback(w http.ResponseWriter, r *http.Request) {
	if s.telegramAuth.BotToken == "" {
		writeError(w, http.StatusNotImplemented, errors.New("telegram auth not configured"))
		return
	}

	tgStore, ok := s.store.(telegramStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("telegram auth not supported by store"))
		return
	}

	// Collect all query params into a map.
	params := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	// Verify signature.
	if err := verifyTelegramAuth(params, s.telegramAuth.BotToken); err != nil {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("telegram auth failed: %w", err))
		return
	}

	telegramID := strings.TrimSpace(params["id"])
	firstName := params["first_name"]
	lastName := params["last_name"]
	username := params["username"]

	// Check whitelist.
	ctx := r.Context()
	entry, err := tgStore.CheckTelegramWhitelist(ctx, telegramID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !entry.Allowed {
		// Redirect to login page with error.
		http.Redirect(w, r, "/?error=not_authorized", http.StatusFound)
		return
	}

	// Prefer the whitelist display name when present.
	displayName := strings.TrimSpace(entry.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(firstName + " " + lastName)
	}

	// Find or create user.
	user, err := tgStore.UpsertTelegramUser(ctx, telegramID, username, displayName, entry.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Create session and redirect.
	session, err := s.store.CreateSession(ctx, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	s.setSessionCookie(w, r, session)
	http.Redirect(w, r, "/", http.StatusFound)
}
