package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrTokenInvalid is returned for an unknown, used or expired set-password link.
var ErrTokenInvalid = errors.New("invalid or expired link")

// WelcomeTokenTTL is how long the set-password link of a welcome e-mail works.
const WelcomeTokenTTL = 72 * time.Hour

// CreatePasswordToken makes a one-time link token with which the account sets its own password.
// Earlier tokens of the account stop working. Only the hash is stored.
func (s *Service) CreatePasswordToken(ctx context.Context, userID int64, ttl time.Duration) (string, time.Time, error) {
	token := randomToken(32)
	now := s.Now().UTC()
	exp := now.Add(ttl)
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = ? OR expires_at < ?`, userID, now.Format(time.RFC3339)); err != nil {
		return "", exp, err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO password_tokens(token_hash, user_id, expires_at) VALUES(?,?,?)`,
		hashToken(token), userID, exp.Format(time.RFC3339))
	return token, exp, err
}

// PasswordTokenUser returns the enabled account a valid token belongs to.
func (s *Service) PasswordTokenUser(ctx context.Context, token string) (*User, error) {
	if len(token) != 64 {
		return nil, ErrTokenInvalid
	}
	var id int64
	var exp string
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, expires_at FROM password_tokens WHERE token_hash = ?`, hashToken(token)).Scan(&id, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339, exp); err != nil || !s.Now().Before(t) {
		return nil, ErrTokenInvalid
	}
	u, err := s.UserByID(ctx, id)
	if err != nil || !u.Enabled {
		return nil, ErrTokenInvalid
	}
	return u, nil
}

// SetPasswordWithToken sets the password of the token's account and uses the token up.
// Existing sessions of the account end.
func (s *Service) SetPasswordWithToken(ctx context.Context, token, password string) (*User, error) {
	u, err := s.PasswordTokenUser(ctx, token)
	if err != nil {
		return nil, err
	}
	h, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, h, u.ID); err != nil {
		return nil, err
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = ?`, u.ID)
	s.DeleteUserSessions(ctx, u.ID)
	return u, nil
}

// LinkExtension links an account to an extension (one account per extension).
func (s *Service) LinkExtension(ctx context.Context, userID, extID int64) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET extension_id = NULL WHERE extension_id = ? AND id <> ?`, extID, userID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET extension_id = ? WHERE id = ?`, extID, userID)
	return err
}
