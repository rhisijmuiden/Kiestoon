// Package auth handles users, passwords, sessions, TOTP-based MFA and recovery codes.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// SessionTTL is how long a session stays valid without logging in again.
const SessionTTL = 12 * time.Hour

// DesktopTTL is how long a session of the Windows softphone stays valid. It slides: every day the
// softphone is used it is extended again, so a desk that is used daily never has to sign in again.
const DesktopTTL = 30 * 24 * time.Hour

// MethodDesktop marks a session of the Windows softphone. It may only use the user portal API.
const MethodDesktop = "desktop"

// MethodTrusted marks a password session that skipped the second factor because it signed in from a
// trusted network. It loses that status as soon as it is used from elsewhere.
const MethodTrusted = "trusted-network"

// Issuer is the name shown in authenticator apps.
const Issuer = "Kiestoon"

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidCode        = errors.New("invalid code")
	ErrWeakPassword       = errors.New("password must be at least 12 characters")
	ErrNoSession          = errors.New("no valid session")
)

// User is an account that can sign in to the portal.
type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	TOTPEnabled bool   `json:"mfaEnabled"`
	Language    string `json:"language"`
	// Theme is the UI theme: "system" (follow the device), "dark" or "light".
	Theme string `json:"theme"`
	Email string `json:"email"`
	// ExtensionID links the account to its phone (for the user portal).
	ExtensionID *int64 `json:"extensionId"`
	// SSOProvider is the identity provider the account signs in with (0: local password).
	SSOProvider int64  `json:"ssoProvider"`
	Enabled     bool   `json:"enabled"`
	LastLoginAt string `json:"lastLoginAt"`
	CreatedAt   string `json:"createdAt"`
	totpSecret  string
	hash        string
}

// Roles, from most to least rights.
var Roles = []string{"superadmin", "admin", "reception", "user"}

// RoleRank orders roles: a higher rank has more rights.
func RoleRank(role string) int {
	switch role {
	case "superadmin":
		return 4
	case "admin":
		return 3
	case "reception":
		return 2
	case "user":
		return 1
	}
	return 0
}

// IsAdmin reports whether the user may manage the system. Admins must use MFA.
func (u *User) IsAdmin() bool { return u.Role == "admin" || u.Role == "superadmin" }

// IsReception reports whether the user may see and steer live calls, queues and the day/night switch.
func (u *User) IsReception() bool { return RoleRank(u.Role) >= 2 }

// Session is a signed-in browser.
type Session struct {
	User      *User
	MFAPassed bool
	// Method is how the session signed in: password, or sso / sso-mfa (the identity provider
	// already asked for a second factor, so Kiestoon does not).
	Method string
	// Renewed is set when a desktop session was just extended, so the cookie is sent again.
	Renewed   bool
	tokenHash string
}

// Service wraps the database for all authentication operations.
type Service struct {
	DB  *sql.DB
	Now func() time.Time
}

// New returns a Service using the real clock.
func New(db *sql.DB) *Service { return &Service{DB: db, Now: time.Now} }

// HashPassword hashes a password with bcrypt after checking the minimum length.
func HashPassword(pw string) (string, error) {
	if len([]rune(pw)) < 12 {
		return "", ErrWeakPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(h), err
}

// UserCount returns the number of accounts; zero means first-run setup is needed.
func (s *Service) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser adds an account.
func (s *Service) CreateUser(ctx context.Context, username, displayName, password, role, language string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}
	if displayName == "" {
		displayName = username
	}
	if language != "nl" {
		language = "en"
	}
	h, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO users(username, display_name, password_hash, role, language) VALUES(?, ?, ?, ?, ?)`,
		username, displayName, h, role, language)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

const userCols = `id, username, display_name, role, totp_enabled, language, theme, totp_secret, password_hash, email, extension_id,
	sso_provider, enabled, last_login_at, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var totp, enabled int
	var ext sql.NullInt64
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &totp, &u.Language, &u.Theme, &u.totpSecret, &u.hash, &u.Email, &ext,
		&u.SSOProvider, &enabled, &u.LastLoginAt, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.TOTPEnabled, u.Enabled = totp == 1, enabled == 1
	if ext.Valid {
		u.ExtensionID = &ext.Int64
	}
	return u, nil
}

// UserByID loads an account.
func (s *Service) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// dummyHash makes failed logins for unknown users take as long as for known users.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("kiestoon-timing-guard"), 12)

// CheckPassword verifies credentials and returns the user.
func (s *Service) CheckPassword(ctx context.Context, username, password string) (*User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, strings.TrimSpace(username)))
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.hash), []byte(password)) != nil || !u.Enabled {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// SetLanguage stores the user's preferred UI language ("en" or "nl").
func (s *Service) SetLanguage(ctx context.Context, userID int64, lang string) error {
	if lang != "nl" {
		lang = "en"
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET language = ? WHERE id = ?`, lang, userID)
	return err
}

// SetDisplayName changes the name shown for an account.
func (s *Service) SetDisplayName(ctx context.Context, userID int64, name string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, strings.TrimSpace(name), userID)
	return err
}

// SetTheme stores the user's preferred UI theme ("system", "dark" or "light").
func (s *Service) SetTheme(ctx context.Context, userID int64, theme string) error {
	if !ValidTheme(theme) {
		theme = "system"
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET theme = ? WHERE id = ?`, theme, userID)
	return err
}

// ValidTheme reports whether theme is a known UI theme.
func ValidTheme(theme string) bool {
	return theme == "system" || theme == "dark" || theme == "light"
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system's random source is broken; nothing is safe after this
	}
	return hex.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// CreateSession starts a session and returns the raw token for the cookie.
// mfaPassed is true when the user has no MFA yet (they will be sent to enrollment).
func (s *Service) CreateSession(ctx context.Context, u *User, mfaPassed bool) (string, error) {
	return s.CreateSessionMethod(ctx, u, mfaPassed, "password")
}

// CreateSessionMethod starts a session and records how the user signed in.
func (s *Service) CreateSessionMethod(ctx context.Context, u *User, mfaPassed bool, method string) (string, error) {
	ttl := SessionTTL
	if method == MethodDesktop {
		ttl = DesktopTTL
	}
	token := randomToken(32)
	now := s.Now().UTC()
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, mfa_passed, created_at, expires_at, method) VALUES(?, ?, ?, ?, ?, ?)`,
		hashToken(token), u.ID, boolInt(mfaPassed), now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339), method)
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now.Format(time.RFC3339), u.ID)
	}
	return token, err
}

// SessionFromToken loads a valid session for a cookie token.
func (s *Service) SessionFromToken(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	th := hashToken(token)
	var uid int64
	var mfa int
	var expires, method string
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, mfa_passed, expires_at, method FROM sessions WHERE token_hash = ?`, th).
		Scan(&uid, &mfa, &expires, &method)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || s.Now().After(exp) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, th)
		return nil, ErrNoSession
	}
	u, err := s.UserByID(ctx, uid)
	if err != nil || !u.Enabled {
		return nil, ErrNoSession
	}
	sess := &Session{User: u, MFAPassed: mfa == 1, Method: method, tokenHash: th}
	if method == MethodDesktop && exp.Sub(s.Now()) < DesktopTTL-24*time.Hour {
		next := s.Now().UTC().Add(DesktopTTL).Format(time.RFC3339)
		if _, err := s.DB.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, next, th); err == nil {
			sess.Renewed = true
		}
	}
	return sess, nil
}

// MarkMFAPassed records that the session completed the second factor.
func (s *Service) MarkMFAPassed(ctx context.Context, sess *Session) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET mfa_passed = 1 WHERE token_hash = ?`, sess.tokenHash)
	sess.MFAPassed = true
	return err
}

// RequireMFA takes a skipped second factor back: the session must pass MFA before it continues.
func (s *Service) RequireMFA(ctx context.Context, sess *Session) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET mfa_passed = 0, method = 'password' WHERE token_hash = ?`, sess.tokenHash)
	sess.MFAPassed = false
	sess.Method = "password"
	return err
}

// DeleteSession signs the browser out.
func (s *Service) DeleteSession(ctx context.Context, sess *Session) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, sess.tokenHash)
	return err
}

// PurgeExpiredSessions removes old sessions; called periodically.
func (s *Service) PurgeExpiredSessions(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, s.Now().UTC().Format(time.RFC3339))
}

// BeginTOTPEnrollment creates a new (not yet active) TOTP secret and returns the otpauth URL.
func (s *Service) BeginTOTPEnrollment(ctx context.Context, u *User) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: Issuer, AccountName: u.Username, Algorithm: otp.AlgorithmSHA1, Digits: otp.DigitsSix})
	if err != nil {
		return "", "", err
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_secret = ? WHERE id = ? AND totp_enabled = 0`, key.Secret(), u.ID); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ConfirmTOTPEnrollment activates MFA when the code matches and returns fresh recovery codes.
func (s *Service) ConfirmTOTPEnrollment(ctx context.Context, u *User, code string) ([]string, error) {
	fresh, err := s.UserByID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if fresh.TOTPEnabled || fresh.totpSecret == "" {
		return nil, errors.New("no enrollment in progress")
	}
	step, ok := matchTOTP(fresh.totpSecret, code, s.Now())
	if !ok {
		return nil, ErrInvalidCode
	}
	codes := make([]string, 8)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, u.ID); err != nil {
		return nil, err
	}
	for i := range codes {
		codes[i] = newRecoveryCode()
		if _, err := tx.ExecContext(ctx, `INSERT INTO recovery_codes(user_id, code_hash) VALUES(?, ?)`, u.ID, hashToken(normalizeCode(codes[i]))); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET totp_enabled = 1, totp_last_step = ? WHERE id = ?`, step, u.ID); err != nil {
		return nil, err
	}
	return codes, tx.Commit()
}

// VerifySecondFactor accepts a TOTP code or an unused recovery code.
func (s *Service) VerifySecondFactor(ctx context.Context, u *User, code string) error {
	fresh, err := s.UserByID(ctx, u.ID)
	if err != nil {
		return err
	}
	if !fresh.TOTPEnabled {
		return ErrInvalidCode
	}
	code = strings.TrimSpace(code)
	if len(code) == 6 {
		step, ok := matchTOTP(fresh.totpSecret, code, s.Now())
		if !ok {
			return ErrInvalidCode
		}
		// Only accept a step newer than the last one used: a code that was seen once cannot be replayed.
		res, err := s.DB.ExecContext(ctx, `UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, u.ID, step)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return nil
		}
		return ErrInvalidCode
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE recovery_codes SET used = 1 WHERE user_id = ? AND code_hash = ? AND used = 0`,
		u.ID, hashToken(normalizeCode(code)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	return ErrInvalidCode
}

// matchTOTP checks a 6-digit code against the current time step and one step either side
// (to allow for clock drift) and returns the matching step.
func matchTOTP(secret, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 || secret == "" {
		return 0, false
	}
	opts := totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	base := now.Unix() / 30
	for _, d := range []int64{0, -1, 1} {
		step := base + d
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*30, 0), opts)
		if err == nil && ConstantTimeEqual(want, code) {
			return step, true
		}
	}
	return 0, false
}

// ChangePassword replaces the password after checking the current one.
func (s *Service) ChangePassword(ctx context.Context, u *User, current, next string) error {
	fresh, err := s.UserByID(ctx, u.ID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(fresh.hash), []byte(current)) != nil {
		return ErrInvalidCredentials
	}
	h, err := HashPassword(next)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, h, u.ID)
	return err
}

func newRecoveryCode() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	return s[:4] + "-" + s[4:]
}

func normalizeCode(c string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(c), "-", ""))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ConstantTimeEqual compares two strings without leaking timing information.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
