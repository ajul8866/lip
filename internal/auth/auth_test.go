package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	valid := []string{
		"correct horse battery staple",
		"한글비밀번호는충분히길어야합니다",
		"a-long-unique-passphrase",
	}
	for _, password := range valid {
		if err := ValidatePassword(password); err != nil {
			t.Errorf("%q should be valid: %v", password, err)
		}
	}
	invalid := []string{
		"short",
		"123456789012",
		"password1234",
		strings.Repeat("a", 73),
	}
	for _, password := range invalid {
		if err := ValidatePassword(password); err == nil {
			t.Errorf("%q should be rejected", password)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	password := "a sufficiently long passphrase"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == password {
		t.Fatal("hash must not equal password")
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
	if VerifyPassword("", password) {
		t.Fatal("empty hash must not verify")
	}
}

func TestTokens(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if len(token) < 40 || strings.ContainsAny(token, "+/=") {
		t.Fatalf("unexpected token encoding: %q", token)
	}
	if HashToken(token) != hash {
		t.Fatal("hash mismatch")
	}
	token2, hash2, _ := NewSessionToken()
	if token == token2 || hash == hash2 {
		t.Fatal("tokens must be unique")
	}
	if !ConstantTimeEqual(hash, HashToken(token)) {
		t.Fatal("constant time compare failed")
	}
}

func TestNormalizeEmail(t *testing.T) {
	email, err := NormalizeEmail("Analyst@Example.COM")
	if err != nil || email != "Analyst@example.com" {
		t.Fatalf("normalize: %q %v", email, err)
	}
	for _, bad := range []string{"", "nope", "Name <a@b.com>", "a@b.com extra"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestPasswordPolicyErrorIsSentinel(t *testing.T) {
	err := ValidatePassword("short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got %v", err)
	}
}
