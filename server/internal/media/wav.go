// Package media handles audio files: prompts, voicemail messages, greetings and call recordings.
package media

import (
	"encoding/binary"
	"errors"
)

// ErrFormat is returned for audio that is not 8 kHz, 16-bit, mono PCM WAV. The browser converts
// uploads and recordings to this format before sending them, so Kiestoon needs no transcoder.
var ErrFormat = errors.New("audio must be a WAV file: PCM, 16 bit, mono, 8000 Hz")

// MaxPromptBytes limits an upload to about 5 minutes of audio.
const MaxPromptBytes = 5 * 60 * 16000

// CheckWAV validates a WAV file and returns its duration in milliseconds.
func CheckWAV(b []byte) (int64, error) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, ErrFormat
	}
	var fmtOK bool
	pos := 12
	for pos+8 <= len(b) {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(b) {
			if id == "data" && fmtOK {
				size = len(b) - body // some recorders write a wrong data size
			} else {
				return 0, ErrFormat
			}
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return 0, ErrFormat
			}
			f := b[body : body+16]
			format := binary.LittleEndian.Uint16(f[0:2])
			channels := binary.LittleEndian.Uint16(f[2:4])
			rate := binary.LittleEndian.Uint32(f[4:8])
			bits := binary.LittleEndian.Uint16(f[14:16])
			if format != 1 || channels != 1 || rate != 8000 || bits != 16 {
				return 0, ErrFormat
			}
			fmtOK = true
		case "data":
			if !fmtOK {
				return 0, ErrFormat
			}
			return int64(size) * 1000 / 16000, nil
		}
		pos = body + size + size%2
	}
	return 0, ErrFormat
}
