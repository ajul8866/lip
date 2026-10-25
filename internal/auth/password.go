// Package auth implements password policy, hashing, and opaque session
// tokens. Password hashes never leave the store package; tokens are returned
// once and only their SHA-256 digest is persisted.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordBytes = 12
	MaxPasswordBytes = 72
	bcryptCost       = 12
	TokenBytes       = 32
)

var ErrWeakPassword = errors.New("password does not meet policy")

var commonPasswords = map[string]struct{}{
	"password1234": {}, "password12345": {}, "administrator": {},
	"letmein12345": {}, "qwerty123456": {}, "123456789012": {},
	"changeme1234": {}, "welcome12345": {}, "iloveyou1234": {},
	"passwordpassword": {}, "adminadmin12": {}, "lip-lip-lip-lip": {},
}

// ValidatePassword applies NIST-style length rules plus a tiny common-password
// denylist. Composition rules are deliberately not used.
func ValidatePassword(password string) error {
	n := len(password)
	if n < MinPasswordBytes || n > MaxPasswordBytes {
		return fmt.Errorf("%w: must be %d-%d bytes", ErrWeakPassword, MinPasswordBytes, MaxPasswordBytes)
	}
	if utf8.RuneCountInString(password) < MinPasswordBytes {
		return fmt.Errorf("%w: must be at least %d characters", ErrWeakPassword, MinPasswordBytes)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("%w: must not be blank", ErrWeakPassword)
	}
	if _, found := commonPasswords[strings.ToLower(password)]; found {
		return fmt.Errorf("%w: choose a less common password", ErrWeakPassword)
	}
	hasLetter := false
	for _, r := range password {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return fmt.Errorf("%w: must contain a letter", ErrWeakPassword)
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) bool {
	if hash == "" || password == "" || len(password) > MaxPasswordBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func NormalizeEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 254 {
		return "", errors.New("invalid email address")
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw {
		return "", errors.New("invalid email address")
	}
	at := strings.LastIndex(raw, "@")
	if at <= 0 {
		return "", errors.New("invalid email address")
	}
	return raw[:at] + "@" + strings.ToLower(raw[at+1:]), nil
}

// NewSessionToken returns the bearer value and the digest to persist.
func NewSessionToken() (token, hash string, err error) {
	var b [TokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	enc := base64.RawURLEncoding
	token = enc.EncodeToString(b[:])
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// DummyHash is used to spend the same bcrypt time for unknown accounts. It is
// a hash of a public placeholder, not a credential.
const DummyHash = "$2a$12$oanB0w.Crr1kqbpq.QcgK.knwB2qXxe0DOR1sboeCQ6s26VAkKdGa"
