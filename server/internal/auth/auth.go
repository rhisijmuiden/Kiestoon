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
	totpSecret  string
	hash        string
}

// IsAdmin reports whether the user may manage the system. Admins must use MFA.
func (u *User) IsAdmin() bool { return u.Role == "admin" || u.Role == "superadmin" }

// Session is a signed-in browser.
type Session struct {
	User      *User
	MFAPassed bool
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

const userCols = `id, username, display_name, role, totp_enabled, language, totp_secret, password_hash`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var enabled int
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &enabled, &u.Language, &u.totpSecret, &u.hash); err != nil {
		return nil, err
	}
	u.TOTPEnabled = enabled == 1
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
	if bcrypt.CompareHashAndPassword([]byte(u.hash), []byte(password)) != nil {
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
	token := randomToken(32)
	now := s.Now().UTC()
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, mfa_passed, created_at, expires_at) VALUES(?, ?, ?, ?, ?)`,
		hashToken(token), u.ID, boolInt(mfaPassed), now.Format(time.RFC3339), now.Add(SessionTTL).Format(time.RFC3339))
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
	var expires string
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, mfa_passed, expires_at FROM sessions WHERE token_hash = ?`, th).
		Scan(&uid, &mfa, &expires)
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
	if err != nil {
		return nil, ErrNoSession
	}
	return &Session{User: u, MFAPassed: mfa == 1, tokenHash: th}, nil
}

// MarkMFAPassed records that the session completed the second factor.
func (s *Service) MarkMFAPassed(ctx context.Context, sess *Session) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET mfa_passed = 1 WHERE token_hash = ?`, sess.tokenHash)
	sess.MFAPassed = true
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
	if !totp.Validate(strings.TrimSpace(code), fresh.totpSecret) {
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
	if _, err := tx.ExecContext(ctx, `UPDATE users SET totp_enabled = 1 WHERE id = ?`, u.ID); err != nil {
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
	if len(code) == 6 && totp.Validate(code, fresh.totpSecret) {
		return nil
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
