// Package backup makes encrypted, deduplicated backups with restic to FTP, FTPS, SFTP or SCP (SSH)
// servers, through rclone. The root helper runs these commands; the portal only stores the
// targets and the schedule.
//
// A "full" backup reads every file again and is stored without a parent snapshot; an
// "incremental" backup only reads files that changed since the previous snapshot. Restic stores
// every snapshot as a complete picture, so any snapshot can be restored on its own.
package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Target is a remote backup location.
type Target struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // ftp | ftps | sftp | scp
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
	PasswordSet  bool   `json:"passwordSet"`
	PrivateKey   string `json:"privateKey,omitempty"`
	KeySet       bool   `json:"keySet"`
	Path         string `json:"path"`
	ImplicitTLS  bool   `json:"implicitTls"`
	RepoPassword string `json:"repoPassword,omitempty"`
	Enabled      bool   `json:"enabled"`
}

var (
	reHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	rePath = regexp.MustCompile(`^[A-Za-z0-9_./~-]{0,200}$`)
	reUser = regexp.MustCompile(`^[A-Za-z0-9_.@+-]{1,64}$`)
	reSnap = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
)

// Validate checks a target before it is stored or used.
func (t *Target) Validate() error {
	t.Name = strings.TrimSpace(t.Name)
	t.Host = strings.TrimSpace(t.Host)
	if t.Name == "" || len(t.Name) > 60 || strings.ContainsAny(t.Name, "\n\r") {
		return errors.New("name")
	}
	switch t.Kind {
	case "ftp", "ftps", "sftp", "scp":
	default:
		return errors.New("kind")
	}
	if net.ParseIP(t.Host) == nil && !reHost.MatchString(t.Host) {
		return errors.New("host")
	}
	if t.Port == 0 {
		t.Port = t.DefaultPort()
	}
	if t.Port < 1 || t.Port > 65535 {
		return errors.New("port")
	}
	if !reUser.MatchString(t.Username) {
		return errors.New("username")
	}
	if !rePath.MatchString(t.Path) || strings.Contains(t.Path, "..") {
		return errors.New("path")
	}
	if t.Password == "" && (t.PrivateKey == "" || t.Kind == "ftp" || t.Kind == "ftps") {
		return errors.New("password")
	}
	if t.PrivateKey != "" && !strings.Contains(t.PrivateKey, "PRIVATE KEY") {
		return errors.New("privateKey")
	}
	if len(t.RepoPassword) < 12 {
		return errors.New("repoPassword")
	}
	return nil
}

// DefaultPort is the usual port of the protocol.
func (t Target) DefaultPort() int {
	switch t.Kind {
	case "sftp", "scp":
		return 22
	case "ftps":
		if t.ImplicitTLS {
			return 990
		}
	}
	return 21
}

// Paths are what a backup contains.
var Paths = []string{"/var/lib/kiestoon", "/etc/kiestoon"}

// Excludes are left out: the live database (a consistent copy is in staging) and temporary files.
var Excludes = []string{
	"/var/lib/kiestoon/kiestoon.db", "/var/lib/kiestoon/kiestoon.db-wal", "/var/lib/kiestoon/kiestoon.db-shm",
	"/var/lib/kiestoon/update-request.json", "/var/lib/kiestoon/mailq/.*",
}

// StagingDir (inside the data directory) holds the consistent database copy the portal makes
// just before a backup.
const StagingDir = "staging"

// Runner runs restic and rclone.
type Runner struct {
	Restic   string
	Rclone   string
	CacheDir string
}

// NewRunner uses the restic and rclone programs from the PATH.
func NewRunner() *Runner {
	return &Runner{Restic: "restic", Rclone: "rclone", CacheDir: "/var/cache/kiestoon/restic"}
}

func (r *Runner) obscure(ctx context.Context, pw string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Rclone, "obscure", "-")
	cmd.Stdin = strings.NewReader(pw)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rclone obscure: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Env builds the environment for restic: the repository on an rclone remote named "kt" that is
// configured entirely through environment variables, so no credentials are written to disk.
func (r *Runner) Env(ctx context.Context, t Target) ([]string, error) {
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("invalid target: %w", err)
	}
	path := strings.TrimPrefix(t.Path, "/")
	if path == "" {
		path = "kiestoon"
	}
	if strings.HasPrefix(t.Path, "/") {
		path = "/" + path
	}
	env := []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"RESTIC_REPOSITORY=rclone:kt:" + path,
		"RESTIC_PASSWORD=" + t.RepoPassword,
		"RESTIC_CACHE_DIR=" + r.CacheDir,
		"RCLONE_CONFIG_KT_HOST=" + t.Host,
		"RCLONE_CONFIG_KT_PORT=" + strconv.Itoa(t.Port),
		"RCLONE_CONFIG_KT_USER=" + t.Username,
	}
	if t.Password != "" {
		pw, err := r.obscure(ctx, t.Password)
		if err != nil {
			return nil, err
		}
		env = append(env, "RCLONE_CONFIG_KT_PASS="+pw)
	}
	switch t.Kind {
	case "ftp", "ftps":
		env = append(env, "RCLONE_CONFIG_KT_TYPE=ftp")
		if t.Kind == "ftps" {
			if t.ImplicitTLS {
				env = append(env, "RCLONE_CONFIG_KT_TLS=true")
			} else {
				env = append(env, "RCLONE_CONFIG_KT_EXPLICIT_TLS=true")
			}
		}
	case "sftp", "scp":
		// SCP targets are SSH servers too; rclone talks SFTP over the same SSH connection.
		env = append(env, "RCLONE_CONFIG_KT_TYPE=sftp", "RCLONE_CONFIG_KT_SHELL_TYPE=none")
		if t.PrivateKey != "" {
			env = append(env, "RCLONE_CONFIG_KT_KEY_PEM="+strings.ReplaceAll(strings.TrimSpace(t.PrivateKey), "\n", `\n`))
		}
	}
	return env, nil
}

// Result summarises a backup run.
type Result struct {
	SnapshotID string `json:"snapshotId"`
	BytesAdded int64  `json:"bytesAdded"`
	Files      int64  `json:"files"`
}

// Snapshot is a stored backup.
type Snapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"shortId"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Tags     []string  `json:"tags"`
}

func (r *Runner) run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Restic, append([]string{"-o", "rclone.program=" + r.Rclone}, args...)...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 && len(msg) > 400 {
			msg = msg[i+1:]
		}
		if len(msg) > 400 {
			msg = msg[:400]
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, &ResticError{Code: ee.ExitCode(), Message: msg}
		}
		return out, fmt.Errorf("restic: %w: %s", err, msg)
	}
	return out, nil
}

// ResticError is a failed restic command.
type ResticError struct {
	Code    int
	Message string
}

func (e *ResticError) Error() string { return fmt.Sprintf("restic exit %d: %s", e.Code, e.Message) }

// EnsureRepo opens the repository and creates it when it does not exist yet.
func (r *Runner) EnsureRepo(ctx context.Context, env []string) (created bool, err error) {
	_, err = r.run(ctx, env, "cat", "config")
	if err == nil {
		return false, nil
	}
	var re *ResticError
	// Exit code 10: the repository does not exist.
	if errors.As(err, &re) && (re.Code == 10 || strings.Contains(re.Message, "does not exist") || strings.Contains(re.Message, "Is there a repository")) {
		if _, err := r.run(ctx, env, "init"); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, err
}

// Backup makes a snapshot. kind is "full" or "incremental".
func (r *Runner) Backup(ctx context.Context, env []string, kind string) (*Result, error) {
	args := []string{"backup", "--json", "--host", "kiestoon", "--tag", "kiestoon", "--tag", kind, "--one-file-system"}
	if kind == "full" {
		args = append(args, "--force")
	}
	for _, e := range Excludes {
		args = append(args, "--exclude", e)
	}
	for _, p := range Paths {
		if _, err := os.Stat(p); err == nil {
			args = append(args, p)
		}
	}
	out, err := r.run(ctx, env, args...)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m struct {
			Type       string `json:"message_type"`
			SnapshotID string `json:"snapshot_id"`
			DataAdded  int64  `json:"data_added"`
			Files      int64  `json:"total_files_processed"`
		}
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Type == "summary" {
			res.SnapshotID, res.BytesAdded, res.Files = m.SnapshotID, m.DataAdded, m.Files
		}
	}
	return res, nil
}

// Retention says how many snapshots to keep.
type Retention struct {
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

// Forget removes snapshots outside the retention and frees their space.
func (r *Runner) Forget(ctx context.Context, env []string, k Retention) error {
	if k.Daily+k.Weekly+k.Monthly == 0 {
		return nil
	}
	_, err := r.run(ctx, env, "forget", "--tag", "kiestoon", "--prune",
		"--keep-daily", strconv.Itoa(k.Daily), "--keep-weekly", strconv.Itoa(k.Weekly), "--keep-monthly", strconv.Itoa(k.Monthly))
	return err
}

// Snapshots lists the stored backups, newest first.
func (r *Runner) Snapshots(ctx context.Context, env []string) ([]Snapshot, error) {
	out, err := r.run(ctx, env, "snapshots", "--json", "--tag", "kiestoon")
	if err != nil {
		return nil, err
	}
	var s []Snapshot
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, err
	}
	for i := range s {
		if len(s[i].ID) >= 8 {
			s[i].ShortID = s[i].ID[:8]
		}
	}
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	if s == nil {
		s = []Snapshot{}
	}
	return s, nil
}

// RestoreTo extracts a snapshot into dir.
func (r *Runner) RestoreTo(ctx context.Context, env []string, snapshot, dir string) error {
	if snapshot != "latest" && !reSnap.MatchString(snapshot) {
		return errors.New("invalid snapshot id")
	}
	_, err := r.run(ctx, env, "restore", snapshot, "--target", dir)
	return err
}
