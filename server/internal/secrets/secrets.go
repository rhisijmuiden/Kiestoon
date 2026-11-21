// Package secrets encrypts small secrets (API keys of cloud services) before they are stored in
// the database. The key is a random 256-bit file in the data directory, so a copy of the database
// alone (an export, a support dump) does not reveal the secrets. The file lives in <data>/secrets,
// which the standby of a high-availability pair replicates and backups include, so both can read
// the secrets too.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// prefix marks a sealed value, so a plain value from an older version is still recognised.
const prefix = "kts1:"

// Box seals and opens secrets with the key in Path (created on first use).
type Box struct {
	Path string

	mu  sync.Mutex
	key []byte
}

func (b *Box) load() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.key != nil {
		return b.key, nil
	}
	k, err := os.ReadFile(b.Path)
	if errors.Is(err, os.ErrNotExist) {
		k = make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(b.Path), 0o750); err != nil {
			return nil, err
		}
		tmp := b.Path + ".tmp"
		if err := os.WriteFile(tmp, k, 0o600); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, b.Path); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(k) != 32 {
		return nil, errors.New("secrets: bad key file")
	}
	b.key = k
	return k, nil
}

func (b *Box) aead() (cipher.AEAD, error) {
	k, err := b.load()
	if err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

// Seal encrypts plain (AES-256-GCM). name binds the value to its purpose, so a sealed value cannot
// be moved to another setting. An empty plain stays empty.
func (b *Box) Seal(name, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	a, err := b.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := a.Seal(nonce, nonce, []byte(plain), []byte(name))
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open decrypts a value from Seal. A value without the prefix is returned as it is.
func (b *Box) Open(name, sealed string) (string, error) {
	if !strings.HasPrefix(sealed, prefix) {
		return sealed, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(sealed, prefix))
	if err != nil {
		return "", err
	}
	a, err := b.aead()
	if err != nil {
		return "", err
	}
	if len(raw) < a.NonceSize() {
		return "", errors.New("secrets: short value")
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(name))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
