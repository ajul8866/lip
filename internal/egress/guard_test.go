package egress

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFailClosedWithoutRules(t *testing.T) {
	guard, err := New(Options{Safe: true})
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	if guard.AllowsHost("example.com", "443") {
		t.Fatal("empty allowlist must deny")
	}
	if err := guard.CheckTarget(context.Background(), "example.com", "443"); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}

func TestAllowlistedLoopbackRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	guard, err := New(Options{Safe: true, Allowlist: []string{address}})
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := guard.Client(5 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("allowed request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSafeModeBlocksPublicIP(t *testing.T) {
	guard, err := New(Options{Safe: true, Allowlist: []string{"93.184.216.34:443"}})
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	if err := guard.CheckTarget(context.Background(), "93.184.216.34", "443"); !errors.Is(err, ErrDenied) {
		t.Fatalf("public IP should be denied in safe mode: %v", err)
	}
}

func TestMetadataAlwaysBlocked(t *testing.T) {
	guard, err := New(Options{Safe: false, Allowlist: []string{"169.254.169.254:80", "[fd00:ec2::254]:80"}})
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	if err := guard.CheckTarget(context.Background(), "169.254.169.254", "80"); !errors.Is(err, ErrDenied) {
		t.Fatalf("metadata IP should be denied: %v", err)
	}
}

func TestURLCredentialsAndSchemes(t *testing.T) {
	guard, _ := New(Options{Safe: false, Allowlist: []string{"example.com:443"}})
	for _, raw := range []string{
		"https://user@example.com/",
		"file:///etc/passwd",
		"gopher://example.com:443/",
		"https://example.com.evil.test/",
	} {
		if _, err := guard.CheckURL(context.Background(), raw); err == nil {
			t.Errorf("%q should be denied", raw)
		}
	}
	if _, err := guard.CheckURL(context.Background(), "https://example.com/path"); err != nil {
		t.Fatalf("allowed URL denied: %v", err)
	}
}

func TestRedirectIsRevalidated(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect target should not be reached")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	redirectAddr := strings.TrimPrefix(redirect.URL, "http://")
	guard, err := New(Options{Safe: true, Allowlist: []string{redirectAddr}})
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, redirect.URL, nil)
	if _, err := guard.Client(5 * time.Second).Do(req); err == nil {
		t.Fatal("redirect to a non-allowlisted host must fail")
	}
}

func TestInvalidRuleFailsStartup(t *testing.T) {
	for _, rule := range []string{"example.com", "example.com:notaport", "*", "*.example.com:443"} {
		_, err := New(Options{Safe: true, Allowlist: []string{rule}})
		if err == nil {
			t.Errorf("rule %q should be rejected in safe mode", rule)
		}
	}
	if _, err := New(Options{Safe: false, Allowlist: []string{"*.example.com:443"}}); err != nil {
		t.Fatalf("wildcard should be valid outside safe mode: %v", err)
	}
}
