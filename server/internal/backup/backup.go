// Package backup makes encrypted, deduplicated backups with restic to FTP, FTPS, SFTP or SCP (SSH)
// servers, S3 object storage, WebDAV (Nextcloud, ownCloud), SMB shares and Microsoft OneDrive, through rclone. The root helper runs these commands; the portal only stores the
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
	"slices"
	"strconv"
	"strings"
	"time"
)

// Target is a remote backup location.
type Target struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // ftp | ftps | sftp | scp | s3 | webdav | smb | onedrive
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
	// OneDrive is the Microsoft account of a "onedrive" target.
	OneDrive *OneDrive `json:"onedrive,omitempty"`
	Extra
}

// Extra holds the settings only some protocols have (stored as JSON).
type Extra struct {
	// Provider is the S3 provider (AWS, Minio, Wasabi, Ceph, Other …) or the WebDAV vendor
	// (nextcloud, owncloud, other).
	Provider string `json:"provider,omitempty"`
	// Region of an S3 bucket.
	Region string `json:"region,omitempty"`
	// Domain is the Windows domain of an SMB user.
	Domain string `json:"domain,omitempty"`
	// Plain uses http instead of https (S3 or WebDAV on the own network only).
	Plain bool `json:"plain,omitempty"`
}

// S3Providers are the S3 providers rclone knows and the UI offers.
var S3Providers = []string{"AWS", "Minio", "Wasabi", "Ceph", "DigitalOcean", "Scaleway", "Hetzner", "Cloudflare", "Other"}

// WebDAVVendors are the WebDAV servers rclone has special support for.
var WebDAVVendors = []string{"nextcloud", "owncloud", "sharepoint-ntlm", "other"}

var (
	reHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	rePath = regexp.MustCompile(`^[A-Za-z0-9_./~-]{0,200}$`)
	// WebDAV paths contain the user name, which may be an e-mail address.
	reDAVPath = regexp.MustCompile(`^[A-Za-z0-9_./~@+-]{0,250}$`)
	reUser    = regexp.MustCompile(`^[A-Za-z0-9_.@+-]{1,64}$`)
	reS3Key   = regexp.MustCompile(`^[A-Za-z0-9_+/=.-]{1,128}$`)
	reRegion  = regexp.MustCompile(`^[a-z0-9-]{0,40}$`)
	reDomain  = regexp.MustCompile(`^[A-Za-z0-9_.-]{0,64}$`)
	reSnap    = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
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
		t.OneDrive = nil
		t.Extra = Extra{}
	case "s3", "webdav", "smb":
		t.OneDrive = nil
		return t.validateObject()
	case "onedrive":
		t.Extra = Extra{}
		if t.OneDrive == nil {
			return errors.New("clientId")
		}
		if err := t.OneDrive.Validate(); err != nil {
			return err
		}
		t.Host, t.Port, t.Username, t.Password, t.PrivateKey, t.ImplicitTLS = "graph.microsoft.com", 443, "", "", "", false
		if !rePath.MatchString(t.Path) || strings.Contains(t.Path, "..") {
			return errors.New("path")
		}
		if len(t.RepoPassword) < 12 {
			return errors.New("repoPassword")
		}
		return nil
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

// validateObject checks S3, WebDAV and SMB targets.
func (t *Target) validateObject() error {
	t.PrivateKey, t.ImplicitTLS = "", false
	if net.ParseIP(t.Host) == nil && !reHost.MatchString(t.Host) {
		return errors.New("host")
	}
	if t.Port == 0 {
		t.Port = t.DefaultPort()
	}
	if t.Port < 1 || t.Port > 65535 {
		return errors.New("port")
	}
	if strings.Contains(t.Path, "..") || strings.ContainsAny(t.Path, "\n\r") {
		return errors.New("path")
	}
	switch t.Kind {
	case "s3":
		t.Domain = ""
		if !slices.Contains(S3Providers, t.Provider) {
			return errors.New("provider")
		}
		if !reS3Key.MatchString(t.Username) {
			return errors.New("username")
		}
		if t.Password == "" || len(t.Password) > 128 || strings.ContainsAny(t.Password, "\n\r") {
			return errors.New("password")
		}
		if !reRegion.MatchString(t.Region) {
			return errors.New("region")
		}
		// bucket/prefix: the bucket name rules of S3.
		bucket, _, _ := strings.Cut(strings.Trim(t.Path, "/"), "/")
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(bucket) || !rePath.MatchString(t.Path) {
			return errors.New("path")
		}
	case "webdav":
		t.Region, t.Domain = "", ""
		if t.Provider == "" {
			t.Provider = "other"
		}
		if !slices.Contains(WebDAVVendors, t.Provider) {
			return errors.New("provider")
		}
		if !reUser.MatchString(t.Username) {
			return errors.New("username")
		}
		if t.Password == "" {
			return errors.New("password")
		}
		if !reDAVPath.MatchString(t.Path) {
			return errors.New("path")
		}
	case "smb":
		t.Region, t.Provider, t.Plain = "", "", false
		if !reUser.MatchString(t.Username) {
			return errors.New("username")
		}
		if t.Password == "" {
			return errors.New("password")
		}
		if !reDomain.MatchString(t.Domain) {
			return errors.New("domain")
		}
		// share/folder: the first part is the share.
		share, _, _ := strings.Cut(strings.Trim(t.Path, "/"), "/")
		if share == "" || !rePath.MatchString(t.Path) {
			return errors.New("path")
		}
	}
	if len(t.RepoPassword) < 12 {
		return errors.New("repoPassword")
	}
	return nil
}

// DefaultPort is the usual port of the protocol.
func (t Target) DefaultPort() int {
	switch t.Kind {
	case "s3", "webdav":
		if t.Plain {
			return 80
		}
		return 443
	case "smb":
		return 445
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
	// Downloaded speech models and phone firmware can be downloaded again.
	"/var/lib/kiestoon/speech", "/var/lib/kiestoon/firmware",
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
	if t.Kind == "onedrive" {
		o := t.OneDrive
		if o.AccessToken == "" || o.DriveID == "" {
			return nil, errors.New("onedrive_not_connected")
		}
		env := []string{
			"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/root",
			"RESTIC_REPOSITORY=rclone:kt:" + strings.TrimPrefix(path, "/"),
			"RESTIC_PASSWORD=" + t.RepoPassword,
			"RESTIC_CACHE_DIR=" + r.CacheDir,
			"RCLONE_CONFIG_KT_TYPE=onedrive",
			"RCLONE_CONFIG_KT_CLIENT_ID=" + o.ClientID,
			"RCLONE_CONFIG_KT_TOKEN=" + o.rcloneToken(),
			"RCLONE_CONFIG_KT_DRIVE_ID=" + o.DriveID,
			"RCLONE_CONFIG_KT_DRIVE_TYPE=" + o.DriveType,
			"RCLONE_CONFIG_KT_AUTH_URL=" + o.authority() + "/authorize",
			"RCLONE_CONFIG_KT_TOKEN_URL=" + o.authority() + "/token",
		}
		if o.ClientSecret != "" {
			env = append(env, "RCLONE_CONFIG_KT_CLIENT_SECRET="+o.ClientSecret)
		}
		return env, nil
	}
	if t.Kind == "s3" || t.Kind == "webdav" || t.Kind == "smb" {
		return r.objectEnv(ctx, t)
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

// objectEnv configures the rclone remote for S3, WebDAV and SMB.
func (r *Runner) objectEnv(ctx context.Context, t Target) ([]string, error) {
	scheme := "https"
	if t.Plain {
		scheme = "http"
	}
	hostPort := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	env := []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"RESTIC_PASSWORD=" + t.RepoPassword,
		"RESTIC_CACHE_DIR=" + r.CacheDir,
	}
	repo := strings.Trim(t.Path, "/")
	switch t.Kind {
	case "s3":
		env = append(env, "RCLONE_CONFIG_KT_TYPE=s3", "RCLONE_CONFIG_KT_PROVIDER="+t.Provider,
			"RCLONE_CONFIG_KT_ACCESS_KEY_ID="+t.Username, "RCLONE_CONFIG_KT_SECRET_ACCESS_KEY="+t.Password,
			"RCLONE_CONFIG_KT_ENDPOINT="+scheme+"://"+hostPort,
			// Never create public objects; restic needs nothing else.
			"RCLONE_CONFIG_KT_ACL=private", "RCLONE_CONFIG_KT_NO_CHECK_BUCKET=true")
		if t.Region != "" {
			env = append(env, "RCLONE_CONFIG_KT_REGION="+t.Region)
		}
		if t.Provider != "AWS" {
			env = append(env, "RCLONE_CONFIG_KT_FORCE_PATH_STYLE=true")
		}
	case "webdav":
		pw, err := r.obscure(ctx, t.Password)
		if err != nil {
			return nil, err
		}
		env = append(env, "RCLONE_CONFIG_KT_TYPE=webdav", "RCLONE_CONFIG_KT_URL="+scheme+"://"+hostPort+"/",
			"RCLONE_CONFIG_KT_VENDOR="+t.Provider, "RCLONE_CONFIG_KT_USER="+t.Username, "RCLONE_CONFIG_KT_PASS="+pw)
	case "smb":
		pw, err := r.obscure(ctx, t.Password)
		if err != nil {
			return nil, err
		}
		env = append(env, "RCLONE_CONFIG_KT_TYPE=smb", "RCLONE_CONFIG_KT_HOST="+t.Host, "RCLONE_CONFIG_KT_PORT="+strconv.Itoa(t.Port),
			"RCLONE_CONFIG_KT_USER="+t.Username, "RCLONE_CONFIG_KT_PASS="+pw)
		if t.Domain != "" {
			env = append(env, "RCLONE_CONFIG_KT_DOMAIN="+t.Domain)
		}
	}
	env = append(env, "RESTIC_REPOSITORY=rclone:kt:"+repo)
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
