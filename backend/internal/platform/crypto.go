package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SecretBox struct {
	gcm cipher.AEAD
}

func LoadOrCreateSecretBox(dataDir string) (*SecretBox, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dataDir, ".settings-key")
	raw, err := os.ReadFile(keyPath)
	var key []byte
	if err == nil {
		key, err = hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("invalid .settings-key file at %s", keyPath)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)), 0o600); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{gcm: gcm}, nil
}

func (s *SecretBox) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *SecretBox) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "enc:v1:") {
		return "", errors.New("unsupported ciphertext prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "enc:v1:"))
	if err != nil {
		return "", err
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := s.gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
