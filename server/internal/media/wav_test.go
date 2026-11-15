package media

import (
	"encoding/binary"
	"testing"
)

func wav(rate uint32, channels uint16, samples int) []byte {
	data := samples * 2 * int(channels)
	b := make([]byte, 44+data)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+data))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], channels)
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[28:], rate*2*uint32(channels))
	binary.LittleEndian.PutUint16(b[32:], 2*channels)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(data))
	return b
}

func TestCheckWAV(t *testing.T) {
	ms, err := CheckWAV(wav(8000, 1, 16000))
	if err != nil || ms != 2000 {
		t.Fatalf("got %d %v", ms, err)
	}
	if _, err := CheckWAV(wav(44100, 1, 100)); err == nil {
		t.Fatal("44.1 kHz accepted")
	}
	if _, err := CheckWAV(wav(8000, 2, 100)); err == nil {
		t.Fatal("stereo accepted")
	}
	if _, err := CheckWAV([]byte("not a wav file at all, really not, no no no no no")); err == nil {
		t.Fatal("garbage accepted")
	}
}
