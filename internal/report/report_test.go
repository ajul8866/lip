package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/lip-project/lip/internal/store"
)

func TestRenderEscapesAndOrders(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	data := Data{
		AppName: "LIP",
		Project: store.Project{ID: "p1", Name: "Acme <script>alert(1)</script>", Description: "line one\nline two"},
		Assets:  []store.Asset{{ID: "a1", Kind: "domain", Value: "example.com|x", Source: "manual", FirstSeen: now}},
		Findings: []store.Finding{
			{ID: "f1", Title: "High <b>thing</b>", Description: "desc ![pixel](https://evil.example/pixel)", Severity: "high", Status: "open", Source: "manual", CreatedAt: now, Evidence: []byte(`{"note":"<img src=x onerror=alert(1)>"}`)},
			{ID: "f2", Title: "Critical", Description: "desc", Severity: "critical", Status: "open", Source: "llm", CreatedAt: now},
		},
		Tasks:       []store.Task{{ID: "t1", Title: "task", Tool: "inventory_summary", RiskLevel: "passive", Status: "succeeded", CreatedAt: now}},
		Approvals:   []store.Approval{{ID: "ap1", TaskID: "t1", Action: "task.execute", Status: "approved", CreatedAt: now}},
		GeneratedAt: now, GeneratedBy: "Ada <ada@example.test>", SafeLocalOnly: true,
		MaxConcurrent: 2, Disclaimer: "Validate <everything>.",
	}
	var buf bytes.Buffer
	if err := Render(&buf, data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>") || strings.Contains(out, "<img") {
		t.Fatalf("unsafe HTML survived: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") || !strings.Contains(out, "&lt;img") {
		t.Fatalf("expected escaped HTML: %s", out)
	}
	if strings.Contains(out, "](https://evil.example") {
		t.Fatal("remote Markdown image syntax survived")
	}
	if strings.Index(out, "Critical") > strings.Index(out, "High") {
		t.Fatal("critical findings must render first")
	}
	if !strings.Contains(out, "Approval decisions") {
		t.Fatal("approval section missing")
	}
	if strings.Contains(out, "example.com|x") {
		t.Fatal("table pipe was not escaped")
	}
}
