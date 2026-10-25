package store

import "testing"

func TestTaskRunnable(t *testing.T) {
	approved := "user"
	cases := []struct {
		task Task
		want bool
	}{
		{Task{Status: "queued", RiskLevel: "passive"}, true},
		{Task{Status: "queued", RiskLevel: "active"}, false},
		{Task{Status: "queued", RiskLevel: "active", ApprovedBy: &approved}, true},
		{Task{Status: "awaiting_approval", RiskLevel: "passive"}, false},
		{Task{Status: "running", RiskLevel: "passive"}, false},
		{Task{Status: "queued", RiskLevel: "destructive", ApprovedBy: &approved}, true},
	}
	for _, tc := range cases {
		if got := tc.task.Runnable(); got != tc.want {
			t.Errorf("%+v: got %v want %v", tc.task, got, tc.want)
		}
	}
}

func TestValidEnums(t *testing.T) {
	for _, role := range Roles {
		if !ValidRole(role) {
			t.Errorf("role %q invalid", role)
		}
	}
	if ValidRole("root") {
		t.Error("root should not be valid")
	}
	for _, severity := range Severities {
		if !ValidSeverity(severity) {
			t.Errorf("severity %q invalid", severity)
		}
	}
	if ValidProviderKind("mock") {
		t.Error("mock provider must not be accepted")
	}
}
