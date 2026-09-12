package engine

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// MasterKeyEnvName is the environment variable that holds the master encryption key.
const MasterKeyEnvName = "DDCORE_SECRET_KEY"

// MasterKey derives a 32-byte key from DDCORE_SECRET_KEY using SHA-256.
func MasterKey() ([]byte, error) {
	val := strings.TrimSpace(os.Getenv(MasterKeyEnvName))
	if val == "" {
		return nil, cerr.Validation("Vault secret key ({0}) is not configured on this site", MasterKeyEnvName)
	}
	hash := sha256.Sum256([]byte(val))
	return hash[:], nil
}

// EncryptVault encrypts a plaintext string using AES-256-GCM.
func EncryptVault(key []byte, plaintext string) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

// DecryptVault decrypts ciphertext using AES-256-GCM.
func DecryptVault(key, ciphertext, nonce []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", cerr.Validation("Failed to decrypt vault secret: authentication error")
	}
	return string(plaintext), nil
}
