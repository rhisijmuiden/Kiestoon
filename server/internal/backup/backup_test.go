package backup

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func TestObjectTargets(t *testing.T) {
	r := &Runner{Rclone: "/bin/false", CacheDir: "/tmp/c"}
	s3 := Target{Name: "s3", Kind: "s3", Host: "s3.eu-central-1.amazonaws.com", Username: "AKIAEXAMPLE", Password: "secret/key+1",
		Path: "my-bucket/kiestoon", RepoPassword: "0123456789abcdef", Extra: Extra{Provider: "AWS", Region: "eu-central-1"}}
	env, err := r.Env(context.Background(), s3)
	if err != nil {
		t.Fatal(err)
	}
	m := envMap(env)
	if m["RCLONE_CONFIG_KT_TYPE"] != "s3" || m["RCLONE_CONFIG_KT_SECRET_ACCESS_KEY"] != "secret/key+1" ||
		m["RCLONE_CONFIG_KT_ENDPOINT"] != "https://s3.eu-central-1.amazonaws.com:443" || m["RESTIC_REPOSITORY"] != "rclone:kt:my-bucket/kiestoon" ||
		m["RCLONE_CONFIG_KT_REGION"] != "eu-central-1" {
		t.Fatalf("s3 env: %v", m)
	}
	if _, ok := m["RCLONE_CONFIG_KT_FORCE_PATH_STYLE"]; ok {
		t.Fatal("AWS uses virtual-host style")
	}
	bad := s3
	bad.Path = "My_Bucket"
	if bad.Validate() == nil {
		t.Fatal("invalid bucket accepted")
	}
	bad = s3
	bad.Provider = "Evil"
	if bad.Validate() == nil {
		t.Fatal("unknown provider accepted")
	}
	smb := Target{Name: "nas", Kind: "smb", Host: "nas.local", Username: "backup", Password: "pw", Path: "backups/kiestoon",
		RepoPassword: "0123456789abcdef", Extra: Extra{Domain: "OFFICE"}}
	if err := smb.Validate(); err != nil || smb.Port != 445 {
		t.Fatalf("smb: %v %d", err, smb.Port)
	}
	dav := Target{Name: "nc", Kind: "webdav", Host: "cloud.example.com", Username: "jan@example.com", Password: "pw",
		Path: "/remote.php/dav/files/jan@example.com/kiestoon", RepoPassword: "0123456789abcdef", Extra: Extra{Provider: "nextcloud"}}
	if err := dav.Validate(); err != nil || dav.Port != 443 {
		t.Fatalf("webdav: %v", err)
	}
	if !slices.Contains(Excludes, "/var/lib/kiestoon/speech") {
		t.Fatal("speech models must not be backed up")
	}
}
