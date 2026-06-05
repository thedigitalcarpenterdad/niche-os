package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/store/sqlite/storedb"
)

const insertOrIgnoreIdentitySQL = `INSERT OR IGNORE INTO identities (id, user_id, provider, provider_subject, email, created_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6)`

func (s *Store) UpsertIdentityUser(ctx context.Context, input store.UpsertIdentityUserInput) (store.User, error) {
	provider := strings.TrimSpace(input.Provider)
	subject := strings.TrimSpace(input.ProviderSubject)
	if provider == "" || subject == "" {
		return store.User{}, errors.New("identity provider and subject are required")
	}
	// Fast path: identity already resolves to an existing user.
	row, err := s.q.GetUserByIdentityProviderSubject(ctx, storedb.GetUserByIdentityProviderSubjectParams{Provider: provider, ProviderSubject: subject})
	if err == nil {
		return s.hydrateUserNotificationSettings(ctx, storeUserFromIdentityProviderSubject(row))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.User{}, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)

	user := store.User{
		ID:          newID("usr"),
		Kind:        "human",
		DisplayName: strings.TrimSpace(input.DisplayName),
		Handle:      "",
		AvatarURL:   strings.TrimSpace(input.AvatarURL),
		CreatedAt:   now(),
	}
	if user.DisplayName == "" {
		user.DisplayName = strings.TrimSpace(input.Email)
	}
	if user.DisplayName == "" {
		user.DisplayName = provider + ":" + subject
	}
	if err := qtx.InsertHumanUser(ctx, storedb.InsertHumanUserParams{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		AvatarUrl:   user.AvatarURL,
		CreatedAt:   user.CreatedAt,
	}); err != nil {
		return store.User{}, err
	}

	// Insert the identity. INSERT OR IGNORE so a pre-existing (provider, subject)
	// row does not crash with a UNIQUE constraint violation.
	res, err := tx.ExecContext(ctx, insertOrIgnoreIdentitySQL,
		newID("idn"), user.ID, provider, subject, strings.TrimSpace(input.Email), user.CreatedAt)
	if err != nil {
		return store.User{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Identity already existed. Find which user it points to.
		var existingUserID string
		if err := tx.QueryRowContext(ctx,
			`SELECT user_id FROM identities WHERE provider = ?1 AND provider_subject = ?2`,
			provider, subject).Scan(&existingUserID); err != nil {
			return store.User{}, err
		}
		var userCount int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM users WHERE id = ?1`, existingUserID).Scan(&userCount); err != nil {
			return store.User{}, err
		}
		if userCount > 0 {
			// A real prior account owns this identity. Discard the user we just
			// created and return the existing account.
			_ = tx.Rollback()
			row2, err2 := s.q.GetUserByIdentityProviderSubject(ctx, storedb.GetUserByIdentityProviderSubjectParams{Provider: provider, ProviderSubject: subject})
			if err2 != nil {
				return store.User{}, err2
			}
			return s.hydrateUserNotificationSettings(ctx, storeUserFromIdentityProviderSubject(row2))
		}
		// Orphaned identity (its user row is gone). Relink it to the user we just
		// created so login succeeds going forward.
		if _, err := tx.ExecContext(ctx,
			`UPDATE identities SET user_id = ?1, email = ?2 WHERE provider = ?3 AND provider_subject = ?4`,
			user.ID, strings.TrimSpace(input.Email), provider, subject); err != nil {
			return store.User{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		// Last-resort race handling: a concurrent request may have created the
		// identity between our checks and commit.
		_ = tx.Rollback()
		if row2, err2 := s.q.GetUserByIdentityProviderSubject(ctx, storedb.GetUserByIdentityProviderSubjectParams{Provider: provider, ProviderSubject: subject}); err2 == nil {
			return s.hydrateUserNotificationSettings(ctx, storeUserFromIdentityProviderSubject(row2))
		}
		return store.User{}, err
	}
	return s.hydrateUserNotificationSettings(ctx, user)
}
