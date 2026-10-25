// Package i18n serves the UI string bundles. The JSON files are the single
// source of truth: the Next.js build copies them, and the API serves them for
// clients that prefer runtime delivery.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

type Bundle struct {
	strings map[string]map[string]string
}

func New() (*Bundle, error) {
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("read locales: %w", err)
	}
	b := &Bundle{strings: map[string]map[string]string{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(entry.Name(), ".json")
		raw, err := localesFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var values map[string]string
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("locale %s is empty", locale)
		}
		b.strings[locale] = values
	}
	if len(b.strings) == 0 {
		return nil, fmt.Errorf("no locales embedded")
	}
	return b, nil
}

func (b *Bundle) Supported() []string {
	out := make([]string, 0, len(b.strings))
	for locale := range b.strings {
		out = append(out, locale)
	}
	sort.Strings(out)
	return out
}

// Get returns a locale's strings. Unknown locales fall back to English, then
// to any available locale (keeping startup resilient if en is removed by a
// maintainer).
func (b *Bundle) Get(locale string) (string, map[string]string, bool) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if values, ok := b.strings[locale]; ok {
		return locale, values, true
	}
	if values, ok := b.strings["en"]; ok {
		return "en", values, false
	}
	for key, values := range b.strings {
		return key, values, false
	}
	return "en", map[string]string{}, false
}

func (b *Bundle) Has(locale string) bool {
	_, ok := b.strings[strings.ToLower(locale)]
	return ok
}

func (b *Bundle) Validate() error {
	en, ok := b.strings["en"]
	if !ok {
		return fmt.Errorf("en locale is required")
	}
	for locale, values := range b.strings {
		for key := range en {
			if _, ok := values[key]; !ok {
				return fmt.Errorf("locale %s is missing key %q", locale, key)
			}
		}
	}
	return nil
}
