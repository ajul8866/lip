package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedUIRouting(t *testing.T) {
	handler := Handler()
	cases := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/dashboard", http.StatusOK},
		{"/settings", http.StatusOK},
		{"/missing.js", http.StatusNotFound},
		{"/../../go.mod", http.StatusNotFound},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, rec.Code, tc.want)
		}
		if tc.want == http.StatusOK && !strings.Contains(strings.ToLower(rec.Body.String()), "lip") {
			t.Errorf("%s: body does not look like LIP", tc.path)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: got %d", rec.Code)
	}
}
