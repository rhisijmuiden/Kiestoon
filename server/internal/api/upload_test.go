package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestUploadErrCode(t *testing.T) {
	for err, want := range map[error]string{
		&http.MaxBytesError{Limit: 1}:                  "file_too_large",
		fmt.Errorf("write: %w", syscall.ENOSPC):        "disk_full",
		fmt.Errorf("read: %w", os.ErrDeadlineExceeded): "upload_timeout",
		fmt.Errorf("read: %w", io.ErrUnexpectedEOF):    "upload_interrupted",
		&os.PathError{Op: "open", Err: syscall.EACCES}: "storage_permission",
		fmt.Errorf("something else"):                   "internal",
	} {
		if got, _ := uploadErrCode(err); got != want {
			t.Errorf("%v: %s, want %s", err, got, want)
		}
	}
}

// slowBody sends n chunks of 64 KB with a pause in between.
type slowBody struct {
	n     int
	pause time.Duration
}

func (b *slowBody) Read(p []byte) (int, error) {
	if b.n == 0 {
		return 0, io.EOF
	}
	b.n--
	time.Sleep(b.pause)
	if len(p) > 64<<10 {
		p = p[:64<<10]
	}
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// A slow upload outlasts the server's ReadTimeout (30 s in production, 1 s here): the firmware
// handler extends the read deadline, so it still arrives.
func TestSlowFirmwareUpload(t *testing.T) {
	c, s := adminClient(t)
	ts := httptest.NewUnstartedServer(SecurityHeaders(s.Routes()))
	ts.Config.ReadHeaderTimeout = time.Second
	ts.Config.ReadTimeout = time.Second
	ts.Start()
	defer ts.Close()
	from, _ := url.Parse(c.url)
	req, _ := http.NewRequest("POST", ts.URL+"/api/firmware/upload?vendor=yealink&name=T42U-108.86.0.60.rom", &slowBody{n: 30, pause: 100 * time.Millisecond})
	req.Header.Set("X-Kiestoon-Upload", "1")
	req.Header.Set("Content-Type", "application/octet-stream")
	for _, ck := range c.hc.Jar.Cookies(from) {
		req.AddCookie(ck)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("slow upload: %d %s", resp.StatusCode, b)
	}
}
