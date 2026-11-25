package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword hashes a plaintext password with Argon2id and a random 16-byte salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("argon2id$%s$%s", b64Salt, b64Hash), nil
}

// VerifyPassword checks whether plaintext matches an Argon2id encoded hash in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false, errors.New("invalid password hash format")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, err
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, err
	}

	if len(salt) != saltLen || len(expectedHash) != argonKeyLen {
		return false, errors.New("invalid password hash length")
	}
	actualHash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}
