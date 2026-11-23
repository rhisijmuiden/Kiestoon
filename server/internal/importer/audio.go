package importer

import (
	"bytes"
	"encoding/binary"
	"path"
	"strings"
)

// The browser converts every recording to telephone audio (8 kHz WAV) before it is stored as a
// prompt, like any other upload. Browsers decode WAV (PCM), MP3, Ogg and FLAC, but not the raw
// telephone formats Asterisk keeps its sounds in. NormalizeAudio turns those into PCM WAV first.

// AudioPlayable reports whether a browser can decode the file (after NormalizeAudio).
func AudioPlayable(data []byte) bool {
	switch {
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		f, _, _, _ := wavFormat(data)
		return f == 1 || f == 3 || f == 0xfffe
	case bytes.HasPrefix(data, []byte("ID3")), len(data) > 1 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		return true
	case bytes.HasPrefix(data, []byte("OggS")), bytes.HasPrefix(data, []byte("fLaC")):
		return true
	}
	return false
}

// NormalizeAudio converts raw G.711, signed linear and G.711 WAV files to 16-bit PCM WAV. Other
// data is returned as it is.
func NormalizeAudio(name string, data []byte) []byte {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".ulaw", ".ul", ".pcm", ".mu":
		return pcmWAV(decodeG711(data, ulaw), 8000)
	case ".alaw", ".al":
		return pcmWAV(decodeG711(data, alaw), 8000)
	case ".sln", ".raw":
		return pcmWAV(slin(data), 8000)
	case ".sln16":
		return pcmWAV(slin(data), 16000)
	case ".sln48":
		return pcmWAV(slin(data), 48000)
	}
	if len(data) > 44 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		f, ch, rate, body := wavFormat(data)
		if (f == 6 || f == 7) && body != nil {
			s := decodeG711(body, alaw)
			if f == 7 {
				s = decodeG711(body, ulaw)
			}
			if ch == 2 {
				s = mono(s)
			}
			return pcmWAV(s, int(rate))
		}
	}
	return data
}

// wavFormat returns the format tag, channels, rate and the data chunk of a WAV file.
func wavFormat(b []byte) (format, channels uint16, rate uint32, data []byte) {
	pos := 12
	for pos+8 <= len(b) {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(b) {
			size = len(b) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				format = binary.LittleEndian.Uint16(b[body:])
				channels = binary.LittleEndian.Uint16(b[body+2:])
				rate = binary.LittleEndian.Uint32(b[body+4:])
			}
		case "data":
			return format, channels, rate, b[body : body+size]
		}
		pos = body + size + size%2
	}
	return format, channels, rate, nil
}

func mono(s []int16) []int16 {
	out := make([]int16, len(s)/2)
	for i := range out {
		out[i] = int16((int32(s[2*i]) + int32(s[2*i+1])) / 2)
	}
	return out
}

func slin(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

func pcmWAV(s []int16, rate int) []byte {
	var buf bytes.Buffer
	n := len(s) * 2
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+n))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint32(rate))
	binary.Write(&buf, binary.LittleEndian, uint32(rate*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(n))
	binary.Write(&buf, binary.LittleEndian, s)
	return buf.Bytes()
}

func decodeG711(b []byte, f func(byte) int16) []int16 {
	out := make([]int16, len(b))
	for i, c := range b {
		out[i] = f(c)
	}
	return out
}

func ulaw(u byte) int16 {
	u = ^u
	t := (int16(u&0x0f) << 3) + 0x84
	t <<= (u & 0x70) >> 4
	if u&0x80 != 0 {
		return 0x84 - t
	}
	return t - 0x84
}

func alaw(a byte) int16 {
	a ^= 0x55
	t := int16(a&0x0f) << 4
	seg := (a & 0x70) >> 4
	switch seg {
	case 0:
		t += 8
	case 1:
		t += 0x108
	default:
		t += 0x108
		t <<= seg - 1
	}
	if a&0x80 != 0 {
		return t
	}
	return -t
}
