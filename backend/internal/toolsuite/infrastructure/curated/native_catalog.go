package curated

import (
	"fmt"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// The new native operations are reviewed host contracts, not re-exported
// Provider facade schemas. workspaceId is an optional *requested* Workspace:
// actual authority comes from the authenticated Workspace/TargetScope checks.
// Durable SSH Job calls require an explicit SandboxID until an authoritative
// TaskID→Sandbox association is implemented; neither ID is an auth token.
const workspaceRefField = `"workspaceId":{"type":"string","pattern":"^wrk_[0-9a-f]{32}$"}`
const sandboxRefField = `"sandboxId":{"type":"string","pattern":"^sbx_[0-9a-f]{32}$"}`

const nativeReadOutput = `{"type":"object","additionalProperties":false,"required":["path","content","startLine","endLine","truncated"],"properties":{"path":{"type":"string"},"content":{"type":"string"},"startLine":{"type":"integer","minimum":0},"endLine":{"type":"integer","minimum":0},"truncated":{"type":"boolean"}}}`
const nativeSearchOutput = `{"type":"object","additionalProperties":false,"required":["mode","matches","truncated"],"properties":{"mode":{"type":"string","enum":["files","content"]},"matches":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string"},"line":{"type":"integer","minimum":1},"text":{"type":"string"}}}},"truncated":{"type":"boolean"}}}`
const nativeEditOutput = `{"type":"object","additionalProperties":false,"required":["path","operation","replacements","bytesBefore","bytesAfter"],"properties":{"path":{"type":"string"},"operation":{"type":"string","enum":["create","replace","delete"]},"replacements":{"type":"integer","minimum":0},"bytesBefore":{"type":"integer","minimum":0},"bytesAfter":{"type":"integer","minimum":0}}}`
const nativeShellOutput = `{"type":"object","additionalProperties":false,"required":["exitCode","stdout","stderr","timedOut","truncated"],"properties":{"exitCode":{"type":"integer","minimum":-1},"stdout":{"type":"string"},"stderr":{"type":"string"},"timedOut":{"type":"boolean"},"truncated":{"type":"boolean"}}}`
const nativeJobOutput = `{"type":"object","additionalProperties":false,"required":["taskId","state","cancelRequested"],"properties":{"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"},"state":{"type":"string","enum":["running","completed","failed","cancelled","unknown"]},"unit":{"type":"string"},"cancelRequested":{"type":"boolean"},"exitCode":{"type":"integer","minimum":0,"maximum":255}}}`
const nativeLogOutput = `{"type":"object","additionalProperties":false,"required":["taskId","stream","content","cursor","truncated"],"properties":{"taskId":{"type":"string"},"stream":{"type":"string","enum":["stdout","stderr"]},"content":{"type":"string"},"cursor":{"type":"integer","minimum":0},"truncated":{"type":"boolean"}}}`

type nativeMetadata struct {
	name        string
	title       string
	description string
	input       string
	output      string
	tags        []string
}

var reviewedNativeCatalog = map[string]nativeMetadata{
	"workspace.read": {
		name: "read", title: "Read workspace file",
		description: "Read a bounded range of lines from an authorized Workspace using its selected execution environment.",
		input:       `{"type":"object","additionalProperties":false,"required":["path"],"properties":{` + workspaceRefField + `,"path":{"type":"string","minLength":1,"maxLength":4096},"offset":{"type":"integer","minimum":1,"maximum":1000000000},"limit":{"type":"integer","minimum":1,"maximum":2000}}}`,
		output:      nativeReadOutput, tags: []string{"workspace", "filesystem", "read"},
	},
	"workspace.search": {
		name: "search", title: "Search workspace",
		description: "Find Workspace files or bounded text without invoking a shell.",
		input:       `{"type":"object","additionalProperties":false,"required":["mode"],"properties":{` + workspaceRefField + `,"mode":{"type":"string","enum":["files","content"]},"path":{"type":"string","maxLength":4096},"query":{"type":"string","maxLength":4096},"pattern":{"type":"string","maxLength":1024},"regex":{"type":"boolean"},"caseSensitive":{"type":"boolean"},"maxResults":{"type":"integer","minimum":1,"maximum":500}}}`,
		output:      nativeSearchOutput, tags: []string{"workspace", "filesystem", "search"},
	},
	"workspace.edit": {
		name: "edit", title: "Edit workspace file",
		description: "Create, deterministically replace or delete a regular file in an authorized Workspace.",
		input:       `{"type":"object","additionalProperties":false,"required":["path","operation"],"properties":{` + workspaceRefField + `,"path":{"type":"string","minLength":1,"maxLength":4096},"operation":{"type":"string","enum":["create","replace","delete"]},"content":{"type":"string"},"edits":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["oldText","newText"],"properties":{"oldText":{"type":"string"},"newText":{"type":"string"},"expectedReplacements":{"type":"integer","minimum":1,"maximum":1000}}}}}}`,
		output:      nativeEditOutput, tags: []string{"workspace", "filesystem", "write"},
	},
	"workspace.shell": {
		name: "shell", title: "Run workspace command",
		description: "Execute a bounded command in a verified Sandbox Workspace. The selected isolation is enforced by the backend, not by a Tool label.",
		input:       `{"type":"object","additionalProperties":false,"required":["command"],"properties":{` + workspaceRefField + `,"command":{"type":"string","minLength":1,"maxLength":65536},"cwd":{"type":"string","maxLength":4096},"timeoutMs":{"type":"integer","minimum":1,"maximum":120000}}}`,
		output:      nativeShellOutput, tags: []string{"workspace", "process", "shell"},
	},
	"jobs.submit": {
		name: "submit", title: "Submit durable SSH job",
		description: "Submit a bounded, idempotent command to an explicitly authorized SSH Sandbox; uncertain remote acknowledgement is never replayed automatically.",
		input:       `{"type":"object","additionalProperties":false,"required":["sandboxId","idempotencyKey","command"],"properties":{` + sandboxRefField + `,"idempotencyKey":{"type":"string","pattern":"^[0-9a-f]{32}$"},"command":{"type":"string","minLength":1,"maxLength":65536}}}`,
		output:      nativeJobOutput, tags: []string{"jobs", "ssh", "execute"},
	},
	"jobs.status": {
		name: "status", title: "Observe SSH job",
		description: "Read the state of a task belonging to the specified authorized SSH Sandbox without retrying it.",
		input:       `{"type":"object","additionalProperties":false,"required":["sandboxId","taskId"],"properties":{` + sandboxRefField + `,"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"}}}`,
		output:      nativeJobOutput, tags: []string{"jobs", "ssh", "observe"},
	},
	"jobs.logs": {
		name: "logs", title: "Read SSH job logs",
		description: "Read bounded output from an authorized SSH Sandbox task using an explicit cursor.",
		input:       `{"type":"object","additionalProperties":false,"required":["sandboxId","taskId"],"properties":{` + sandboxRefField + `,"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"},"stream":{"type":"string","enum":["stdout","stderr"]},"cursor":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":65536}}}`,
		output:      nativeLogOutput, tags: []string{"jobs", "ssh", "observe"},
	},
	"jobs.result": {
		name: "result", title: "Inspect SSH job result",
		description: "Read the durable terminal result of a specified authorized SSH task; never replay unknown work.",
		input:       `{"type":"object","additionalProperties":false,"required":["sandboxId","taskId"],"properties":{` + sandboxRefField + `,"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"}}}`,
		output:      nativeJobOutput, tags: []string{"jobs", "ssh", "observe"},
	},
	"jobs.cancel": {
		name: "cancel", title: "Request SSH job cancellation",
		description: "Request cancellation of the specified authorized SSH task. Request acceptance is not proof of terminal cancellation.",
		input:       `{"type":"object","additionalProperties":false,"required":["sandboxId","taskId"],"properties":{` + sandboxRefField + `,"taskId":{"type":"string","pattern":"^sjob_[0-9a-f]{32}$"}}}`,
		output:      nativeJobOutput, tags: []string{"jobs", "ssh", "cancel"},
	},
}

// PlannedNativeCatalogTools constructs the exact host-reviewed Tool metadata
// for the reserved IDs. It never publishes them to the live Catalog or
// activates a Sandbox. Stage E must check each ID for collision and migrate
// every old Profile/Policy reference atomically before these can be supplied.
func PlannedNativeCatalogTools(createdAt time.Time) ([]catalog.Tool, error) {
	entries, err := PlannedNativeTools()
	if err != nil {
		return nil, err
	}
	tools := make([]catalog.Tool, 0, len(entries))
	if len(reviewedNativeCatalog) != len(entries) {
		return nil, fmt.Errorf("reviewed native catalog: registry size mismatch")
	}
	for _, entry := range entries {
		meta, ok := reviewedNativeCatalog[entry.SourceKey]
		if !ok {
			return nil, fmt.Errorf("reviewed native catalog: missing %s", entry.SourceKey)
		}
		input, err := catalog.NewSchema([]byte(meta.input))
		if err != nil {
			return nil, fmt.Errorf("%s input schema: %w", entry.SourceKey, err)
		}
		output, err := catalog.NewSchema([]byte(meta.output))
		if err != nil {
			return nil, fmt.Errorf("%s output schema: %w", entry.SourceKey, err)
		}
		tool, err := catalog.NewToolFromSource(entry.ToolID, entry.Source,
			catalog.ToolSpec{SourceKey: entry.SourceKey, Name: meta.name, Title: meta.title,
				Description: meta.description, InputSchema: input, OutputSchema: &output, Tags: meta.tags}, createdAt)
		if err != nil {
			return nil, fmt.Errorf("create native %s: %w", entry.SourceKey, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}
