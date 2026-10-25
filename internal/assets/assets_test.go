package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndDedupe(t *testing.T) {
	input := `
# comment
domain example.com
url https://example.com/path#fragment
ip 192.0.2.10
cidr 192.0.2.0/24
example.com:8443
not a valid value
domain example.com
`
	res, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(res.Items) != 5 {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Skipped != 1 {
		t.Fatalf("skipped = %d", res.Skipped)
	}
	foundHostPort := false
	for _, item := range res.Items {
		if item.Kind == "host" && item.Value == "example.com:8443" {
			foundHostPort = true
		}
		if item.Kind == "url" && strings.Contains(item.Value, "#") {
			t.Fatalf("fragment should be removed: %s", item.Value)
		}
	}
	if !foundHostPort {
		t.Fatal("host:port was not preserved")
	}
}

func TestNormalizeExplicitKind(t *testing.T) {
	cases := []struct {
		kind, value, want string
		ok                bool
	}{
		{"domain", "Example.COM.", "example.com", true},
		{"ip", "2001:0db8::1", "2001:db8::1", true},
		{"host", "Example.com:443", "example.com:443", true},
		{"url", "https://user@example.com", "", false},
		{"cidr", "192.0.2.9/24", "192.0.2.0/24", true},
		{"nope", "example.com", "", false},
	}
	for _, tc := range cases {
		got, ok := Normalize(tc.kind, tc.value)
		if ok != tc.ok || got != tc.want {
			t.Errorf("Normalize(%q,%q) = %q,%v want %q,%v", tc.kind, tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReadLocalConfinesPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "assets.txt"), []byte("domain example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadLocal(root, "assets.txt")
	if err != nil || !strings.Contains(string(data), "example.com") {
		t.Fatalf("read: %q %v", data, err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocal(root, outside); err == nil {
		t.Fatal("absolute escape should be rejected")
	}
	if _, err := ReadLocal(root, "../outside.txt"); err == nil {
		t.Fatal("relative escape should be rejected")
	}

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocal(root, "link.txt"); err == nil {
		t.Fatal("symlink escape should be rejected")
	}
}
