package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"regexp"
	"slices"
	"strings"
)

// ErrLastSuperadmin protects against locking everyone out.
var ErrLastSuperadmin = errors.New("the last superadmin cannot be removed or demoted")

// ErrUsername is returned for an empty or invalid username.
var ErrUsername = errors.New("invalid username")

var reUsername = regexp.MustCompile(`^[A-Za-z0-9._@+-]{2,64}$`)

// Users returns every account.
func (s *Service) Users(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY display_name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UserByExtension returns the account linked to an extension.
func (s *Service) UserByExtension(ctx context.Context, extID int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE extension_id = ? LIMIT 1`, extID))
}

// UserBySSO finds the account of an identity provider subject.
func (s *Service) UserBySSO(ctx context.Context, provider int64, subject string) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE sso_provider = ? AND sso_subject = ?`, provider, subject))
}

// UserByEmailOrName finds an account by e-mail address or username (to link a first SSO sign-in).
func (s *Service) UserByEmailOrName(ctx context.Context, email string) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE (email <> '' AND email = ? COLLATE NOCASE)
		OR username = ? LIMIT 1`, email, email))
}

// UserInput is what an administrator sets on an account.
type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	ExtensionID *int64 `json:"extensionId"`
	Language    string `json:"language"`
	Enabled     bool   `json:"enabled"`
	// Password is only set for new local accounts or a reset; empty keeps the current one.
	Password string `json:"password"`
}

func (in *UserInput) clean() error {
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Email = strings.TrimSpace(in.Email)
	if !reUsername.MatchString(in.Username) {
		return ErrUsername
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Username
	}
	if len([]rune(in.DisplayName)) > 80 {
		return ErrUsername
	}
	if in.Email != "" {
		if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
			return errors.New("invalid email")
		}
	}
	if !slices.Contains(Roles, in.Role) {
		return errors.New("invalid role")
	}
	if in.Language != "nl" {
		in.Language = "en"
	}
	return nil
}

// SaveUser creates (id 0) or updates an account.
func (s *Service) SaveUser(ctx context.Context, id int64, in UserInput) (*User, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	var ext any
	if in.ExtensionID != nil && *in.ExtensionID > 0 {
		ext = *in.ExtensionID
		// One account per extension.
		if _, err := s.DB.ExecContext(ctx, `UPDATE users SET extension_id = NULL WHERE extension_id = ? AND id <> ?`, *in.ExtensionID, id); err != nil {
			return nil, err
		}
	}
	if id == 0 {
		if in.Password == "" {
			in.Password = randomToken(24) // SSO-only account: nobody knows this password
		}
		h, err := HashPassword(in.Password)
		if err != nil {
			return nil, err
		}
		res, err := s.DB.ExecContext(ctx, `INSERT INTO users(username, display_name, password_hash, role, language, email, extension_id, enabled)
			VALUES(?,?,?,?,?,?,?,?)`, in.Username, in.DisplayName, h, in.Role, in.Language, in.Email, ext, boolInt(in.Enabled))
		if err != nil {
			return nil, err
		}
		id, _ = res.LastInsertId()
		return s.UserByID(ctx, id)
	}
	old, err := s.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if old.Role == "superadmin" && (in.Role != "superadmin" || !in.Enabled) {
		if n, _ := s.countRole(ctx, "superadmin"); n <= 1 {
			return nil, ErrLastSuperadmin
		}
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET username=?, display_name=?, role=?, language=?, email=?, extension_id=?, enabled=?
		WHERE id=?`, in.Username, in.DisplayName, in.Role, in.Language, in.Email, ext, boolInt(in.Enabled), id); err != nil {
		return nil, err
	}
	if in.Password != "" {
		h, err := HashPassword(in.Password)
		if err != nil {
			return nil, err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, h, id); err != nil {
			return nil, err
		}
	}
	if !in.Enabled || in.Password != "" || in.Role != old.Role {
		s.DeleteUserSessions(ctx, id)
	}
	return s.UserByID(ctx, id)
}

func (s *Service) countRole(ctx context.Context, role string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ? AND enabled = 1`, role).Scan(&n)
	return n, err
}

// DeleteUser removes an account.
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	u, err := s.UserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		return err
	}
	if u.Role == "superadmin" {
		if n, _ := s.countRole(ctx, "superadmin"); n <= 1 {
			return ErrLastSuperadmin
		}
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// ResetMFA switches off the second factor; an admin then has to enroll again at the next sign-in.
func (s *Service) ResetMFA(ctx context.Context, id int64) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_enabled = 0, totp_secret = '', totp_last_step = 0 WHERE id = ?`, id); err != nil {
		return err
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, id)
	s.DeleteUserSessions(ctx, id)
	return nil
}

// DeleteUserSessions signs an account out everywhere.
func (s *Service) DeleteUserSessions(ctx context.Context, id int64) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id)
}

// LinkSSO stores the identity provider subject of an account.
func (s *Service) LinkSSO(ctx context.Context, id, provider int64, subject string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET sso_provider = ?, sso_subject = ? WHERE id = ?`, provider, subject, id)
	return err
}

// SetRole changes the role of an account (SSO group mapping on every sign-in).
func (s *Service) SetRole(ctx context.Context, id int64, role string) error {
	if !slices.Contains(Roles, role) {
		return errors.New("invalid role")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}
