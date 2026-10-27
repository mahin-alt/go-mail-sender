package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Secrets are stored as "enc:v1:" + base64(nonce || ciphertext) using AES-256-GCM.
//
// Concept: GCM is "authenticated" encryption. Besides hiding the value it
// detects tampering or a wrong key, so decrypting with the wrong key fails
// loudly instead of returning garbage.
const encPrefix = "enc:v1:"

// ErrNoEncryptionKey means a secret was read or written but no key is configured.
var ErrNoEncryptionKey = errors.New("SETTINGS_ENCRYPTION_KEY is not configured")

// ParseKey decodes a base64 key and checks it is exactly 32 bytes (AES-256).
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("SETTINGS_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("SETTINGS_ENCRYPTION_KEY must decode to 32 bytes, got %d (use: openssl rand -base64 32)", len(key))
	}
	return key, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) == 0 {
		return nil, ErrNoEncryptionKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func encrypt(key []byte, plain string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil) // nonce is prepended
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func decrypt(key []byte, stored string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil || !strings.HasPrefix(stored, encPrefix) || len(raw) < gcm.NonceSize() {
		return "", errors.New("stored secret is malformed")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", errors.New("cannot decrypt stored secret (wrong SETTINGS_ENCRYPTION_KEY?)")
	}
	return string(plain), nil
}
