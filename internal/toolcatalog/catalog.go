// Package toolcatalog is the single source of truth for the bounded tool set
// exposed to the planner and runner. Adding a tool requires a definition here
// and an implementation in the runner; unknown names are rejected.
package toolcatalog

type Definition struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Risk             string `json:"risk"`
	RequiresApproval bool   `json:"requiresApproval"`
	ParameterHint    string `json:"parameterHint"`
}

var definitions = []Definition{
	{
		Name:          "inventory_summary",
		Description:   "Summarize the project's declared asset inventory. No network access.",
		Risk:          "passive",
		ParameterHint: "{}",
	},
	{
		Name:          "synthesize_findings",
		Description:   "Ask the configured LLM to draft findings from declared assets and conversation context. No network scanning.",
		Risk:          "passive",
		ParameterHint: "{}",
	},
	{
		Name:             "http_head",
		Description:      "Send one HEAD request to a declared URL asset. Active, rate-limited, and approval-gated.",
		Risk:             "active",
		RequiresApproval: true,
		ParameterHint:    `{"url":"https://declared-asset.example"}`,
	},
}

var byName = func() map[string]Definition {
	m := make(map[string]Definition, len(definitions))
	for _, d := range definitions {
		m[d.Name] = d
	}
	return m
}()

func All() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	return out
}

func Get(name string) (Definition, bool) {
	d, ok := byName[name]
	return d, ok
}

func Names() []string {
	out := make([]string, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, d.Name)
	}
	return out
}
