package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

// CheckTelegramWhitelist returns the whitelist entry for a Telegram ID.
func (s *Store) CheckTelegramWhitelist(ctx context.Context, telegramID string) (store.TelegramWhitelistEntry, error) {
	telegramID = strings.TrimSpace(telegramID)
	if telegramID == "" {
		return store.TelegramWhitelistEntry{}, errors.New("telegram id is required")
	}
	var entry store.TelegramWhitelistEntry
	var username, displayName, role, workspace sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT telegram_id, telegram_username, display_name, role, workspace
		   FROM telegram_whitelist WHERE telegram_id = ?`, telegramID,
	).Scan(&entry.TelegramID, &username, &displayName, &role, &workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return store.TelegramWhitelistEntry{Allowed: false}, nil
	}
	if err != nil {
		return store.TelegramWhitelistEntry{}, err
	}
	entry.TelegramUsername = username.String
	entry.DisplayName = displayName.String
	entry.Role = role.String
	entry.Workspace = workspace.String
	if entry.Role == "" {
		entry.Role = "member"
	}
	entry.Allowed = true
	return entry, nil
}

// UpsertTelegramUser finds a user by telegram_id (or links/creates one) and
// returns the resulting user. New users are added to the default workspace.
func (s *Store) UpsertTelegramUser(ctx context.Context, telegramID, username, displayName, role string) (store.User, error) {
	telegramID = strings.TrimSpace(telegramID)
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if telegramID == "" {
		return store.User{}, errors.New("telegram id is required")
	}

	// 1. Existing user already linked to this telegram_id.
	var existingID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE telegram_id = ?`, telegramID,
	).Scan(&existingID)
	if err == nil {
		// Keep username fresh.
		if username != "" {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE users SET telegram_username = ? WHERE id = ?`, username, existingID)
		}
		return s.GetUser(ctx, existingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.User{}, err
	}

	// 2. Create a fresh user linked to this telegram_id.
	if displayName == "" {
		if username != "" {
			displayName = "@" + username
		} else {
			displayName = "Telegram " + telegramID
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.User{}, err
	}
	defer tx.Rollback()

	userID := newID("usr")
	createdAt := now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, display_name, avatar_url, created_at, handle, kind, telegram_id, telegram_username)
		 VALUES (?, ?, '', ?, '', 'human', ?, ?)`,
		userID, displayName, createdAt, telegramID, username,
	); err != nil {
		return store.User{}, err
	}
	// Add an identity row for consistency with other providers.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities (id, user_id, provider, provider_subject, email, created_at)
		 VALUES (?, ?, 'telegram', ?, '', ?)`,
		newID("idn"), userID, telegramID, createdAt,
	); err != nil {
		return store.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.User{}, err
	}

	// Ensure workspace membership (default workspace, member role).
	if _, err := s.EnsureDefaultWorkspaceMember(ctx, userID); err != nil {
		return store.User{}, err
	}
	return s.GetUser(ctx, userID)
}
