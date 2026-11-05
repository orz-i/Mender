package application

import (
	"slices"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func NormalizeRead(in workspace.ReadRequest) workspace.ReadRequest {
	if in.Offset == 0 {
		in.Offset = 1
	}
	if in.Limit == 0 {
		in.Limit = 200
	}
	return in
}
func NormalizeSearch(in workspace.SearchRequest) workspace.SearchRequest {
	if in.Path == "" {
		in.Path = "."
	}
	if in.MaxResults == 0 {
		in.MaxResults = 100
	}
	return in
}
func NormalizeEdit(in workspace.EditRequest) workspace.EditRequest {
	in.Edits = slices.Clone(in.Edits)
	for i := range in.Edits {
		if in.Edits[i].ExpectedReplacements == 0 {
			in.Edits[i].ExpectedReplacements = 1
		}
	}
	return in
}
func NormalizeShell(in workspace.ShellRequest) workspace.ShellRequest {
	if in.Cwd == "" {
		in.Cwd = "."
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 30000
	}
	return in
}
