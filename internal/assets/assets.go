// Package assets parses analyst-supplied asset lists and confines local file
// imports to a configured root. Remote imports are performed by the API layer
// through the egress guard.
package assets

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lip-project/lip/internal/store"
)

const (
	MaxItems    = 5000
	MaxValue    = 2048
	MaxFileSize = 10 << 20
)

type Item struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type ParseResult struct {
	Items   []Item
	Skipped int
}

var domainPattern = regexp.MustCompile(`^(?i)(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
var hostPattern = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

// Parse accepts one asset per line, optionally prefixed by a kind
// ("domain example.com"). Blank lines and lines starting with # are ignored.
func Parse(r io.Reader) (ParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	var res ParseResult
	seen := map[string]bool{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.Trim(line, `"'`)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		kind := ""
		value := line
		if len(fields) >= 2 && store.ValidAssetKind(strings.ToLower(fields[0])) {
			kind = strings.ToLower(fields[0])
			value = strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		}
		value = strings.Trim(value, `"'`)
		if len(value) == 0 || len(value) > MaxValue || strings.ContainsAny(value, "\x00\r\n\t") {
			res.Skipped++
			continue
		}
		if kind == "" {
			detected, ok := DetectKind(value)
			if !ok {
				res.Skipped++
				continue
			}
			kind = detected
		}
		value = normalizeValue(kind, value)
		if value == "" {
			res.Skipped++
			continue
		}
		key := kind + "\x00" + strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		res.Items = append(res.Items, Item{Kind: kind, Value: value})
		if len(res.Items) > MaxItems {
			return ParseResult{}, fmt.Errorf("asset list exceeds %d items", MaxItems)
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, fmt.Errorf("read asset list: %w", err)
	}
	return res, nil
}

func DetectKind(value string) (string, bool) {
	if _, _, err := net.ParseCIDR(value); err == nil {
		return "cidr", true
	}
	if ip := net.ParseIP(value); ip != nil {
		return "ip", true
	}
	if u, err := url.Parse(value); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		if u.Hostname() == "" || u.User != nil {
			return "", false
		}
		return "url", true
	}
	if strings.HasPrefix(value, "git@") || strings.HasPrefix(value, "ssh://") {
		return "repo", true
	}

	if h, port, err := net.SplitHostPort(value); err == nil {
		if p, err := strconv.Atoi(port); err == nil && p >= 1 && p <= 65535 && (net.ParseIP(h) != nil || hostPattern.MatchString(h)) {
			return "host", true
		}
	}
	if domainPattern.MatchString(value) {
		return "domain", true
	}
	if hostPattern.MatchString(value) && value != "" {
		return "host", true
	}
	return "", false
}

func normalizeValue(kind, value string) string {
	switch kind {
	case "domain":
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		if !domainPattern.MatchString(value) {
			return ""
		}
	case "url":
		u, err := url.Parse(value)
		if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return ""
		}
		u.Fragment = ""
		value = u.String()
	case "ip":
		ip := net.ParseIP(value)
		if ip == nil {
			return ""
		}
		value = ip.String()
	case "cidr":
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return ""
		}
		value = network.String()
	case "host":
		if h, port, err := net.SplitHostPort(value); err == nil {
			p, err := strconv.Atoi(port)
			if err != nil || p < 1 || p > 65535 || (net.ParseIP(h) == nil && !hostPattern.MatchString(h)) {
				return ""
			}
			return strings.ToLower(h) + ":" + port
		}
		if !hostPattern.MatchString(value) {
			return ""
		}
		value = strings.ToLower(strings.TrimSuffix(value, "."))
	case "repo":
		if len(value) > MaxValue {
			return ""
		}
	}
	return value
}

// ReadLocal reads a UTF-8 asset file while ensuring the resolved path stays
// inside root. Symlinks that escape root are rejected.
func ReadLocal(root, requested string) ([]byte, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, fmt.Errorf("import root: %w", err)
	}
	target := requested
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootAbs, target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("import file not found")
		}
		return nil, fmt.Errorf("resolve import file: %w", err)
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("import file escapes the configured import root")
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("import path is not a regular file")
	}
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("import file exceeds 10 MiB")
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxFileSize+1))
}

// Normalize validates and canonicalizes one explicit kind/value pair.
func Normalize(kind, value string) (string, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !store.ValidAssetKind(kind) {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxValue {
		return "", false
	}
	v := normalizeValue(kind, value)
	return v, v != ""
}
