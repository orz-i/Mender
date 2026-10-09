package localworkspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	relative, _, err := i.resolver.resolveCreate(input.Path)
	if err != nil {
		return toolFailure(err)
	}
	if err := writeNewFile(i.fsRoot, relative, []byte(input.Content)); err != nil {
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
	relative, _, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return toolFailure(err)
	}
	if info.Size() > maxEditFileBytes {
		return toolFailure(fmt.Errorf("file exceeds %d byte edit limit", maxEditFileBytes))
	}
	data, err := i.fsRoot.ReadFile(filepath.FromSlash(relative))
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
	if err := replaceFileAtomically(i.fsRoot, relative, []byte(working), info.Mode().Perm()); err != nil {
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
	relative, _, info, err := i.resolver.resolveExisting(input.Path, false)
	if err != nil {
		return toolFailure(err)
	}
	if err := regularFile(info); err != nil {
		return toolFailure(err)
	}
	if err := i.fsRoot.Remove(filepath.FromSlash(relative)); err != nil {
		return toolFailure(fmt.Errorf("delete workspace file: %w", err))
	}
	return completeJSON(editOutput{
		Path:        relative,
		Operation:   "delete",
		BytesBefore: info.Size(),
	})
}

func writeNewFile(root *os.Root, relative string, content []byte) error {
	name := filepath.FromSlash(relative)
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = root.Remove(name)
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

func replaceFileAtomically(
	root *os.Root,
	relative string,
	content []byte,
	mode os.FileMode,
) error {
	target := filepath.FromSlash(relative)
	temp, tempName, err := createRootTempFile(root, filepath.Dir(target))
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = temp.Close()
		if !success {
			_ = root.Remove(tempName)
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
	if err := root.Rename(tempName, target); err != nil {
		return err
	}
	success = true
	return nil
}

func createRootTempFile(root *os.Root, directory string) (*os.File, string, error) {
	for range 16 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", fmt.Errorf("generate edit temp name: %w", err)
		}
		name := filepath.Join(directory, ".mender-edit-"+hex.EncodeToString(random[:]))
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return nil, "", err
	}
	return nil, "", fmt.Errorf("allocate edit temp file: exhausted collision retries")
}
