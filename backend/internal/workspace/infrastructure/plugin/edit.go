package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const (
	maxEditFileBytes    = 4 * 1024 * 1024
	maxEditContentBytes = 4 * 1024 * 1024
	maxEditReplacements = 64
	maxExpectedReplace  = 1000
)

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

type editOutput struct {
	Path         string `json:"path"`
	Operation    string `json:"operation"`
	Replacements int    `json:"replacements"`
	BytesBefore  int64  `json:"bytesBefore"`
	BytesAfter   int64  `json:"bytesAfter"`
}

func (i *Instance) executeEdit(
	ctx context.Context,
	arguments invocationdomain.JSONValue,
) (invocationdomain.Outcome, error) {
	select {
	case <-ctx.Done():
		return invocationdomain.Outcome{}, ctx.Err()
	default:
	}

	var input editInput
	if err := decodeStrict(arguments.Bytes(), &input, "decode workspace.edit arguments"); err != nil {
		return toolFailure(err)
	}
	if len(input.Content) > maxEditContentBytes {
		return toolFailure(fmt.Errorf("content exceeds %d byte edit limit", maxEditContentBytes))
	}
	if len(input.Edits) > maxEditReplacements {
		return toolFailure(fmt.Errorf("edits contains more than %d replacements", maxEditReplacements))
	}

	switch input.Operation {
	case "create":
		return i.createFile(input)
	case "replace":
		return i.replaceFile(input)
	case "delete":
		return i.deleteFile(input)
	default:
		return toolFailure(fmt.Errorf("unsupported edit operation %q", input.Operation))
	}
}

func (i *Instance) createFile(input editInput) (invocationdomain.Outcome, error) {
	if len(input.Edits) != 0 {
		return toolFailure(fmt.Errorf("create must not contain edits"))
	}
	if !utf8.ValidString(input.Content) {
		return toolFailure(fmt.Errorf("workspace.edit supports UTF-8 text content only"))
	}
	relative, path, err := i.resolver.resolveCreate(input.Path)
	if err != nil {
		return toolFailure(err)
	}
	if err := writeNewFile(path, []byte(input.Content)); err != nil {
		return toolFailure(fmt.Errorf("create workspace file: %w", err))
	}
	return completeJSON(editOutput{
		Path:       relative,
		Operation:  "create",
		BytesAfter: int64(len(input.Content)),
	})
}

func (i *Instance) replaceFile(input editInput) (invocationdomain.Outcome, error) {
	if input.Content != "" {
		return toolFailure(fmt.Errorf("replace must not contain content"))
	}
	if len(input.Edits) == 0 {
		return toolFailure(fmt.Errorf("replace requires at least one edit"))
	}
	relative, path, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return toolFailure(err)
	}
	if info.Size() > maxEditFileBytes {
		return toolFailure(fmt.Errorf("file exceeds %d byte edit limit", maxEditFileBytes))
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is workspace-contained above.
	if err != nil {
		return toolFailure(fmt.Errorf("read workspace file for edit: %w", err))
	}
	if !utf8.Valid(data) {
		return toolFailure(fmt.Errorf("workspace.edit supports UTF-8 text files only"))
	}
	working := string(data)
	replacements := 0
	for index, edit := range input.Edits {
		if edit.OldText == "" {
			return toolFailure(fmt.Errorf("edits[%d].oldText must not be empty", index))
		}
		expected := edit.ExpectedReplacements
		if expected == 0 {
			expected = 1
		}
		if expected < 1 || expected > maxExpectedReplace {
			return toolFailure(fmt.Errorf(
				"edits[%d].expectedReplacements must be between 1 and %d",
				index,
				maxExpectedReplace,
			))
		}
		count := strings.Count(working, edit.OldText)
		if count != expected {
			return toolFailure(fmt.Errorf(
				"edits[%d] expected %d replacement(s) but found %d",
				index,
				expected,
				count,
			))
		}
		working = strings.ReplaceAll(working, edit.OldText, edit.NewText)
		replacements += expected
		if len(working) > maxEditContentBytes {
			return toolFailure(fmt.Errorf("edited content exceeds %d byte limit", maxEditContentBytes))
		}
	}
	if !utf8.ValidString(working) {
		return toolFailure(fmt.Errorf("edited content must remain valid UTF-8"))
	}
	if err := replaceFileAtomically(path, []byte(working), info.Mode().Perm()); err != nil {
		return toolFailure(fmt.Errorf("replace workspace file: %w", err))
	}
	return completeJSON(editOutput{
		Path:         relative,
		Operation:    "replace",
		Replacements: replacements,
		BytesBefore:  int64(len(data)),
		BytesAfter:   int64(len(working)),
	})
}

func (i *Instance) deleteFile(input editInput) (invocationdomain.Outcome, error) {
	if input.Content != "" || len(input.Edits) != 0 {
		return toolFailure(fmt.Errorf("delete must not contain content or edits"))
	}
	relative, path, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return toolFailure(err)
	}
	if err := os.Remove(path); err != nil {
		return toolFailure(fmt.Errorf("delete workspace file: %w", err))
	}
	return completeJSON(editOutput{
		Path:        relative,
		Operation:   "delete",
		BytesBefore: info.Size(),
	})
}

func writeNewFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G304 -- path is workspace-contained.
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

func replaceFileAtomically(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".mender-edit-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	success := false
	defer func() {
		_ = temp.Close()
		if !success {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	success = true
	return nil
}
