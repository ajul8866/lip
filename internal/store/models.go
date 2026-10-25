package store

import (
	"encoding/json"
	"time"
)

// Enumerations mirrored by CHECK constraints in the database. Keep these in
// sync with migrations; tests assert the allowed sets.
var (
	Roles          = []string{"admin", "analyst", "viewer"}
	Locales        = []string{"en", "ko"}
	Severities     = []string{"info", "low", "medium", "high", "critical"}
	FindingStatus  = []string{"open", "triaged", "accepted", "fixed", "false_positive"}
	FindingSources = []string{"manual", "llm", "import"}
	TaskStatuses   = []string{"queued", "awaiting_approval", "running", "succeeded", "failed", "cancelled", "blocked"}
	RiskLevels     = []string{"passive", "active", "destructive"}
	AssetKinds     = []string{"host", "domain", "ip", "cidr", "url", "repo", "other"}
	ProviderKinds  = []string{"ollama", "openai"}
	ApprovalStatus = []string{"pending", "approved", "denied", "expired"}
	AuditResults   = []string{"success", "failure", "denied"}
)

func contains(values []string, v string) bool {
	for _, value := range values {
		if value == v {
			return true
		}
	}
	return false
}

func ValidRole(v string) bool          { return contains(Roles, v) }
func ValidLocale(v string) bool        { return contains(Locales, v) }
func ValidSeverity(v string) bool      { return contains(Severities, v) }
func ValidFindingStatus(v string) bool { return contains(FindingStatus, v) }
func ValidFindingSource(v string) bool { return contains(FindingSources, v) }
func ValidTaskStatus(v string) bool    { return contains(TaskStatuses, v) }
func ValidRisk(v string) bool          { return contains(RiskLevels, v) }
func ValidAssetKind(v string) bool     { return contains(AssetKinds, v) }
func ValidProviderKind(v string) bool  { return contains(ProviderKinds, v) }

type User struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"displayName"`
	PasswordHash       string     `json:"-"`
	Role               string     `json:"role"`
	Locale             string     `json:"locale"`
	MustChangePassword bool       `json:"mustChangePassword"`
	DisabledAt         *time.Time `json:"disabledAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (u User) Disabled() bool { return u.DisabledAt != nil }

type Session struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	TokenHash  string     `json:"-"`
	IP         string     `json:"ip"`
	UserAgent  string     `json:"userAgent"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type Project struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedBy   *string    `json:"createdBy,omitempty"`
	ArchivedAt  *time.Time `json:"archivedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type Conversation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Title     string    `json:"title"`
	CreatedBy *string   `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Message struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversationId"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedBy      *string         `json:"createdBy,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type Task struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"projectId"`
	ConversationID *string         `json:"conversationId,omitempty"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Tool           string          `json:"tool"`
	Parameters     json.RawMessage `json:"parameters"`
	RiskLevel      string          `json:"riskLevel"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	LastError      string          `json:"lastError"`
	Result         json.RawMessage `json:"result"`
	CreatedBy      *string         `json:"createdBy,omitempty"`
	ApprovedBy     *string         `json:"approvedBy,omitempty"`
	StartedAt      *time.Time      `json:"startedAt,omitempty"`
	FinishedAt     *time.Time      `json:"finishedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// Runnable reports whether a task may be claimed by the runner. Active and
// destructive tasks require a recorded human approval regardless of who
// created them.
func (t Task) Runnable() bool {
	if t.Status != "queued" {
		return false
	}
	return t.RiskLevel == "passive" || t.ApprovedBy != nil
}

type Finding struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"projectId"`
	TaskID      *string         `json:"taskId,omitempty"`
	AssetID     *string         `json:"assetId,omitempty"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Severity    string          `json:"severity"`
	Status      string          `json:"status"`
	Source      string          `json:"source"`
	Evidence    json.RawMessage `json:"evidence"`
	CreatedBy   *string         `json:"createdBy,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type Asset struct {
	ID         string          `json:"id"`
	ProjectID  string          `json:"projectId"`
	Kind       string          `json:"kind"`
	Value      string          `json:"value"`
	Source     string          `json:"source"`
	Attributes json.RawMessage `json:"attributes"`
	FirstSeen  time.Time       `json:"firstSeen"`
	LastSeen   time.Time       `json:"lastSeen"`
	CreatedBy  *string         `json:"createdBy,omitempty"`
}

type Approval struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	TaskID      string     `json:"taskId"`
	Action      string     `json:"action"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	RequestedBy *string    `json:"requestedBy,omitempty"`
	DecidedBy   *string    `json:"decidedBy,omitempty"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	DecidedAt   *time.Time `json:"decidedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type AuditEntry struct {
	ID         int64           `json:"id"`
	ActorID    *string         `json:"actorId,omitempty"`
	ActorEmail string          `json:"actorEmail"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType"`
	TargetID   string          `json:"targetId"`
	Result     string          `json:"result"`
	IP         string          `json:"ip"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type Setting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy *string         `json:"updatedBy,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type LLMProvider struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	BaseURL   string          `json:"baseUrl"`
	Model     string          `json:"model"`
	APIKeyEnv string          `json:"apiKeyEnv"`
	Enabled   bool            `json:"enabled"`
	IsDefault bool            `json:"isDefault"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type CountByKey struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type Dashboard struct {
	Projects       int64     `json:"projects"`
	Tasks          int64     `json:"tasks"`
	QueuedTasks    int64     `json:"queuedTasks"`
	RunningTasks   int64     `json:"runningTasks"`
	AwaitingTasks  int64     `json:"awaitingTasks"`
	FailedTasks    int64     `json:"failedTasks"`
	Findings       int64     `json:"findings"`
	OpenFindings   int64     `json:"openFindings"`
	CriticalOpen   int64     `json:"criticalOpen"`
	HighOpen       int64     `json:"highOpen"`
	Assets         int64     `json:"assets"`
	RecentFindings []Finding `json:"recentFindings"`
	RecentTasks    []Task    `json:"recentTasks"`
}
