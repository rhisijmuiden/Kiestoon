// Package unifitalk reads a UniFi Talk backup (a UniFi OS .unifi file) so its phone system can be
// imported into Kiestoon: users with their extension, call groups, call queues, the SIP trunk, phone
// numbers and the smart attendants (voice menus) with their recordings.
package unifitalk

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rhisijmuiden/kiestoon/server/internal/pgdump"
	"io"
	"path"
	"strings"
)

// A UniFi OS backup is AES-256-CBC encrypted with a key that is the same on every console (it
// protects nothing; it only makes the file opaque). The first 16 bytes are the IV, the rest is
// the ciphertext without padding. Decrypted it is a gzip-compressed tar with a backup/ folder;
// a Talk backup holds backup/talk/ with unifi-talk.dump (PostgreSQL custom-format dump),
// config.json and talk_audio/ (MP3 recordings of the attendants, queues and voicemail).
var consoleKey = []byte{
	0xe3, 0x83, 0xb7, 0xc5, 0x36, 0x98, 0xb3, 0x6d, 0x4b, 0xae, 0xa4, 0xed, 0x22, 0x18, 0x1e, 0xf7,
	0x36, 0x76, 0xbf, 0xd5, 0xd5, 0xb9, 0x00, 0x05, 0xd9, 0x84, 0x5f, 0xfd, 0x5d, 0xce, 0x98, 0x5f,
}

// Limits on what is kept from the backup.
const (
	maxDumpBytes  = 256 << 20
	maxAudioFile  = 32 << 20
	maxAudioTotal = 512 << 20
)

// Errors for the user.
var (
	ErrNotBackup = errors.New("not_unifi_backup")
	ErrNoTalk    = errors.New("no_talk_in_backup")
)

// Archive is the Talk part of a backup.
type Archive struct {
	// Metadata of the console that made the backup.
	Console     string // hardware short name, e.g. UDMPRO
	Firmware    string
	TimeZone    string
	CreatedAt   int64 // milliseconds since 1970
	TalkVersion string
	Dump        *pgdump.Dump
	Config      map[string]any
	// Audio maps the path below talk_audio/ (e.g. "ivr/custom_0165b450fc334326.mp3") to the MP3 file.
	Audio map[string][]byte
}

// cbcReader decrypts a stream of whole AES blocks; a trailing partial block is dropped.
type cbcReader struct {
	r   io.Reader
	m   cipher.BlockMode
	buf []byte // decrypted, not yet returned
	in  []byte
	err error
}

func (c *cbcReader) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		if c.err != nil {
			return 0, c.err
		}
		n, err := io.ReadFull(c.r, c.in)
		n -= n % aes.BlockSize
		if n > 0 {
			out := make([]byte, n)
			c.m.CryptBlocks(out, c.in[:n])
			c.buf = out
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				err = io.EOF
			}
			c.err = err
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

// Read reads a UniFi OS backup (.unifi). An already decrypted backup (.tar.gz or .tar) is accepted too.
func Read(r io.Reader) (*Archive, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	head, err := br.Peek(262)
	if err != nil && len(head) < aes.BlockSize*2 {
		return nil, ErrNotBackup
	}
	var plain io.Reader
	switch {
	case head[0] == 0x1f && head[1] == 0x8b:
		plain = br
	case len(head) >= 262 && string(head[257:262]) == "ustar":
		plain = br
	default:
		iv := make([]byte, aes.BlockSize)
		if _, err := io.ReadFull(br, iv); err != nil {
			return nil, ErrNotBackup
		}
		block, _ := aes.NewCipher(consoleKey)
		plain = &cbcReader{r: br, m: cipher.NewCBCDecrypter(block, iv), in: make([]byte, 64<<10)}
	}
	pb := bufio.NewReader(plain)
	var tr *tar.Reader
	if m, _ := pb.Peek(2); len(m) == 2 && m[0] == 0x1f && m[1] == 0x8b {
		gz, err := gzip.NewReader(pb)
		if err != nil {
			return nil, ErrNotBackup
		}
		// Only the first gzip member: the decrypted file ends with padding bytes.
		gz.Multistream(false)
		tr = tar.NewReader(gz)
	} else if m, _ := pb.Peek(262); len(m) == 262 && string(m[257:262]) == "ustar" {
		tr = tar.NewReader(pb)
	} else {
		return nil, ErrNotBackup
	}
	a := &Archive{Audio: map[string][]byte{}}
	var total int64
	var sawTar bool
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// The padding after the archive can make the end look broken; keep what was read.
			if sawTar {
				break
			}
			return nil, ErrNotBackup
		}
		sawTar = true
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		switch {
		case name == "backup/metadata.json":
			var m struct {
				Hardware  string `json:"hardware_shortname"`
				Firmware  string `json:"firmware_version"`
				TimeZone  string `json:"timezone"`
				CreatedAt int64  `json:"created_at"`
			}
			if json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&m) == nil {
				a.Console, a.Firmware, a.TimeZone, a.CreatedAt = m.Hardware, m.Firmware, m.TimeZone, m.CreatedAt
			}
		case name == "backup/package-versions.json":
			var m struct {
				Controllers map[string]string `json:"controllers"`
			}
			if json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&m) == nil {
				a.TalkVersion = m.Controllers["talk"]
			}
		case name == "backup/talk/config.json":
			_ = json.NewDecoder(io.LimitReader(tr, 4<<20)).Decode(&a.Config)
		case name == "backup/talk/unifi-talk.dump":
			if h.Size > maxDumpBytes {
				return nil, fmt.Errorf("talk database too large")
			}
			d, err := pgdump.Read(io.LimitReader(tr, maxDumpBytes))
			if err != nil {
				return nil, err
			}
			a.Dump = d
		case strings.HasPrefix(name, "backup/talk/talk_audio/"):
			rel := strings.TrimPrefix(name, "backup/talk/talk_audio/")
			if h.Size > maxAudioFile || total+h.Size > maxAudioTotal || !strings.HasSuffix(strings.ToLower(rel), ".mp3") {
				continue
			}
			var b bytes.Buffer
			if _, err := io.Copy(&b, io.LimitReader(tr, maxAudioFile)); err != nil {
				return nil, ErrNotBackup
			}
			total += int64(b.Len())
			a.Audio[rel] = b.Bytes()
		}
	}
	if !sawTar {
		return nil, ErrNotBackup
	}
	if a.Dump == nil {
		return nil, ErrNoTalk
	}
	return a, nil
}
