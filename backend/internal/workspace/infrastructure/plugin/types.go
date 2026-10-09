package plugin

// Argument schemas are replicated only for reviewed Governance Fact extraction.
// The actual file/process operations are owned by Sandbox WorkspaceSession.
type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type searchInput struct {
	Mode          string `json:"mode"`
	Path          string `json:"path,omitempty"`
	Query         string `json:"query,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
	MaxResults    int    `json:"maxResults,omitempty"`
}
type editInput struct {
	Path      string     `json:"path"`
	Operation string     `json:"operation"`
	Content   string     `json:"content,omitempty"`
	Edits     []textEdit `json:"edits,omitempty"`
}
type textEdit struct {
	OldText              string `json:"oldText"`
	NewText              string `json:"newText"`
	ExpectedReplacements int    `json:"expectedReplacements,omitempty"`
}
type shellInput struct {
	Command   string `json:"command"`
	Cwd       string `json:"cwd,omitempty"`
	TimeoutMS int64  `json:"timeoutMs,omitempty"`
}
