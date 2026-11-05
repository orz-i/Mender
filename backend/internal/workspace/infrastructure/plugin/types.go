package plugin

import workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"

// Governance extraction and execution share the typed operation contracts.
type readInput = workspace.ReadRequest
type searchInput = workspace.SearchRequest
type editInput = workspace.EditRequest
type shellInput = workspace.ShellRequest
