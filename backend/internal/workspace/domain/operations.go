// Package domain defines Workspace operation data independently of Tool JSON,
// Invocation outcomes and the selected execution environment.
package domain

type Operation string

const (
	Read   Operation = "file.read"
	Search Operation = "file.search"
	Edit   Operation = "file.edit"
	Shell  Operation = "process.shell"
)

// Fields records optional values explicitly supplied by a caller. It preserves
// omission semantics through typed adapters without carrying raw Tool JSON.
type Fields uint16

const (
	FieldOffset Fields = 1 << iota
	FieldLimit
	FieldPath
	FieldQuery
	FieldPattern
	FieldRegex
	FieldCaseSensitive
	FieldMaxResults
	FieldContent
	FieldEdits
	FieldCwd
	FieldTimeoutMS
	FieldExpectedReplacements
)

func (f Fields) Has(field Fields) bool { return f&field != 0 }

type ReadRequest struct {
	CanonicalBytes int    `json:"-"`
	Present        Fields `json:"-"`
	Path           string `json:"path"`
	Offset         int    `json:"offset,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}
type ReadResult struct {
	CanonicalBytes int    `json:"-"`
	Path           string `json:"path"`
	Content        string `json:"content"`
	StartLine      int    `json:"startLine"`
	EndLine        int    `json:"endLine"`
	Truncated      bool   `json:"truncated"`
}
type SearchRequest struct {
	CanonicalBytes int    `json:"-"`
	Present        Fields `json:"-"`
	Mode           string `json:"mode"`
	Path           string `json:"path,omitempty"`
	Query          string `json:"query,omitempty"`
	Pattern        string `json:"pattern,omitempty"`
	Regex          bool   `json:"regex,omitempty"`
	CaseSensitive  bool   `json:"caseSensitive,omitempty"`
	MaxResults     int    `json:"maxResults,omitempty"`
}
type SearchMatch struct {
	Path string  `json:"path"`
	Line int     `json:"line,omitempty"`
	Text *string `json:"text,omitempty"`
}
type SearchResult struct {
	CanonicalBytes int           `json:"-"`
	Mode           string        `json:"mode"`
	Matches        []SearchMatch `json:"matches"`
	Truncated      bool          `json:"truncated"`
}
type TextEdit struct {
	Present              Fields `json:"-"`
	OldText              string `json:"oldText"`
	NewText              string `json:"newText"`
	ExpectedReplacements int    `json:"expectedReplacements,omitempty"`
}
type EditRequest struct {
	CanonicalBytes int        `json:"-"`
	Present        Fields     `json:"-"`
	Path           string     `json:"path"`
	Operation      string     `json:"operation"`
	Content        string     `json:"content,omitempty"`
	Edits          []TextEdit `json:"edits,omitempty"`
}
type EditResult struct {
	CanonicalBytes int    `json:"-"`
	Path           string `json:"path"`
	Operation      string `json:"operation"`
	Replacements   int    `json:"replacements"`
	BytesBefore    int64  `json:"bytesBefore"`
	BytesAfter     int64  `json:"bytesAfter"`
}
type ShellRequest struct {
	CanonicalBytes int    `json:"-"`
	Present        Fields `json:"-"`
	Command        string `json:"command"`
	Cwd            string `json:"cwd,omitempty"`
	TimeoutMS      int64  `json:"timeoutMs,omitempty"`
}
type ShellResult struct {
	CanonicalBytes int    `json:"-"`
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	TimedOut       bool   `json:"timedOut"`
	Truncated      bool   `json:"truncated"`
}

// ToolError is an expected operation failure, distinct from an uncertain
// transport/execution error. Only the Tool adapter creates Invocation outcomes.
type ToolError struct{ Message string }

func (e *ToolError) Error() string { return e.Message }

// InvalidResultError identifies a syntactically valid but schema-incompatible
// external result. It must not be decoded into zero values and published.
type InvalidResultError struct {
	Message string
	// CanonicalBytes preserves result-budget-before-schema ordering without
	// carrying the external wire payload across the typed application port.
	CanonicalBytes int
}

func (e *InvalidResultError) Error() string { return e.Message }

func PresentText(value string) *string { return &value }
func OptionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
