package agentskill

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"gopkg.in/yaml.v3"
)

const (
	defaultMaxFiles                    = 512
	defaultMaxFileBytes          int64 = 4 * 1024 * 1024
	defaultMaxTotalBytes         int64 = 32 * 1024 * 1024
	defaultMaxPathBytes                = 1024
	defaultMaxMetadataEntries          = 64
	defaultMaxMetadataKeyRunes         = 128
	defaultMaxMetadataValueRunes       = 2048
	defaultMaxLicenseRunes             = 1024
	defaultMaxAllowedToolsRunes        = 4096
)

var skillNamePattern = regexp.MustCompile("^[a-z0-9]+(?:-[a-z0-9]+)*$")

// Limits bounds filesystem and metadata ingestion for one Skill directory.
type Limits struct {
	MaxFiles              int
	MaxFileBytes          int64
	MaxTotalBytes         int64
	MaxPathBytes          int
	MaxMetadataEntries    int
	MaxMetadataKeyRunes   int
	MaxMetadataValueRunes int
	MaxLicenseRunes       int
	MaxAllowedToolsRunes  int
}

// DefaultLimits returns Mender's bounded local Skill ingestion policy.
func DefaultLimits() Limits {
	return Limits{
		MaxFiles:              defaultMaxFiles,
		MaxFileBytes:          defaultMaxFileBytes,
		MaxTotalBytes:         defaultMaxTotalBytes,
		MaxPathBytes:          defaultMaxPathBytes,
		MaxMetadataEntries:    defaultMaxMetadataEntries,
		MaxMetadataKeyRunes:   defaultMaxMetadataKeyRunes,
		MaxMetadataValueRunes: defaultMaxMetadataValueRunes,
		MaxLicenseRunes:       defaultMaxLicenseRunes,
		MaxAllowedToolsRunes:  defaultMaxAllowedToolsRunes,
	}
}

type frontmatter struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
}

// LoadDirectory validates and snapshots one Agent Skills directory without
// executing any Skill content.
func LoadDirectory(root string) (skilldomain.Snapshot, error) {
	return LoadDirectoryWithLimits(root, DefaultLimits())
}

// LoadFS validates and snapshots a Skill directory from an fs.FS. It is used
// for immutable embedded/package sources where no host filesystem path is
// required.
func LoadFS(source fs.FS, root string) (skilldomain.Snapshot, error) {
	return LoadFSWithLimits(source, root, DefaultLimits())
}

// LoadFSWithLimits behaves like LoadFS with explicit host-owned ingestion
// bounds.
func LoadFSWithLimits(
	source fs.FS,
	root string,
	limits Limits,
) (skilldomain.Snapshot, error) {
	if source == nil {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: filesystem must not be nil")
	}
	if err := validateLimits(limits); err != nil {
		return skilldomain.Snapshot{}, err
	}
	cleanRoot := pathpkg.Clean(root)
	if cleanRoot == "." || !fs.ValidPath(cleanRoot) {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: root must be a valid relative filesystem path")
	}
	if err := validateFSRoot(source, cleanRoot); err != nil {
		return skilldomain.Snapshot{}, err
	}

	rawSkill, err := readFSResource(source, pathpkg.Join(cleanRoot, "SKILL.md"), limits.MaxFileBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md is required")
		}
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md: %w", err)
	}
	metadata, err := parseAndValidateMetadata(pathpkg.Base(cleanRoot), rawSkill, limits)
	if err != nil {
		return skilldomain.Snapshot{}, err
	}
	resources, totalBytes, err := inventoryFSResources(source, cleanRoot, limits)
	if err != nil {
		return skilldomain.Snapshot{}, err
	}
	return skilldomain.Snapshot{
		Metadata:   metadata,
		Resources:  resources,
		Digest:     snapshotDigest(resources),
		TotalBytes: totalBytes,
	}, nil
}

// LoadDirectoryWithLimits behaves like LoadDirectory with explicit host-owned
// ingestion bounds.
func LoadDirectoryWithLimits(
	root string,
	limits Limits,
) (skilldomain.Snapshot, error) {
	if err := validateLimits(limits); err != nil {
		return skilldomain.Snapshot{}, err
	}

	cleanRoot := filepath.Clean(root)
	info, err := os.Lstat(cleanRoot)
	if err != nil {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: root must not be a symlink")
	}
	if !info.IsDir() {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: root must be a directory")
	}

	skillPath := filepath.Join(cleanRoot, "SKILL.md")
	skillInfo, err := os.Lstat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md is required")
		}
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md: %w", err)
	}
	if skillInfo.Mode()&os.ModeSymlink != 0 || !skillInfo.Mode().IsRegular() {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md must be a regular file")
	}
	if skillInfo.Size() > limits.MaxFileBytes {
		return skilldomain.Snapshot{}, fmt.Errorf(
			"load agent skill: SKILL.md exceeds %d bytes",
			limits.MaxFileBytes,
		)
	}

	rawSkill, err := os.ReadFile(skillPath)
	if err != nil {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: read SKILL.md: %w", err)
	}
	if !utf8.Valid(rawSkill) {
		return skilldomain.Snapshot{}, fmt.Errorf("load agent skill: SKILL.md must be valid UTF-8")
	}

	metadata, err := parseAndValidateMetadata(filepath.Base(cleanRoot), rawSkill, limits)
	if err != nil {
		return skilldomain.Snapshot{}, err
	}

	resources, totalBytes, err := inventoryResources(cleanRoot, limits)
	if err != nil {
		return skilldomain.Snapshot{}, err
	}
	return skilldomain.Snapshot{
		Metadata:   metadata,
		Resources:  resources,
		Digest:     snapshotDigest(resources),
		TotalBytes: totalBytes,
	}, nil
}

func validateLimits(limits Limits) error {
	switch {
	case limits.MaxFiles < 1:
		return fmt.Errorf("load agent skill: max files must be positive")
	case limits.MaxFileBytes < 1:
		return fmt.Errorf("load agent skill: max file bytes must be positive")
	case limits.MaxTotalBytes < limits.MaxFileBytes:
		return fmt.Errorf("load agent skill: max total bytes must be at least max file bytes")
	case limits.MaxPathBytes < 1:
		return fmt.Errorf("load agent skill: max path bytes must be positive")
	case limits.MaxMetadataEntries < 0:
		return fmt.Errorf("load agent skill: max metadata entries must not be negative")
	case limits.MaxMetadataKeyRunes < 1:
		return fmt.Errorf("load agent skill: max metadata key runes must be positive")
	case limits.MaxMetadataValueRunes < 1:
		return fmt.Errorf("load agent skill: max metadata value runes must be positive")
	case limits.MaxLicenseRunes < 1:
		return fmt.Errorf("load agent skill: max license runes must be positive")
	case limits.MaxAllowedToolsRunes < 1:
		return fmt.Errorf("load agent skill: max allowed-tools runes must be positive")
	default:
		return nil
	}
}

func parseAndValidateMetadata(
	directoryName string,
	rawSkill []byte,
	limits Limits,
) (skilldomain.Metadata, error) {
	if !utf8.Valid(rawSkill) {
		return skilldomain.Metadata{}, fmt.Errorf("load agent skill: SKILL.md must be valid UTF-8")
	}
	rawFrontmatter, err := splitFrontmatter(rawSkill)
	if err != nil {
		return skilldomain.Metadata{}, err
	}
	parsed, err := parseFrontmatter(rawFrontmatter)
	if err != nil {
		return skilldomain.Metadata{}, err
	}
	return validateMetadata(directoryName, parsed, limits)
}

func validateFSRoot(source fs.FS, root string) error {
	seen := false
	err := fs.WalkDir(source, root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root {
			return fs.SkipDir
		}
		seen = true
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("root must not be a symlink")
		}
		if !entry.IsDir() {
			return fmt.Errorf("root must be a directory")
		}
		return fs.SkipDir
	})
	if err != nil {
		return fmt.Errorf("load agent skill: root: %w", err)
	}
	if !seen {
		return fmt.Errorf("load agent skill: root not found")
	}
	return nil
}

func readFSResource(source fs.FS, name string, maxBytes int64) ([]byte, error) {
	parent := pathpkg.Dir(name)
	base := pathpkg.Base(name)
	entries, err := fs.ReadDir(source, parent)
	if err != nil {
		return nil, err
	}

	var target fs.DirEntry
	for _, entry := range entries {
		if entry.Name() == base {
			target = entry
			break
		}
	}
	if target == nil {
		return nil, fs.ErrNotExist
	}
	if target.Type()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must not be a symlink", name)
	}
	if target.IsDir() {
		return nil, fmt.Errorf("%s must be a regular file", name)
	}
	info, err := target.Info()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", name)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxBytes)
	}

	file, err := source.Open(name)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxBytes)
	}
	if int64(len(raw)) != info.Size() {
		return nil, fmt.Errorf("%s changed while being inventoried", name)
	}
	return raw, nil
}

func splitFrontmatter(raw []byte) ([]byte, error) {
	firstNewline := bytes.IndexByte(raw, '\n')
	if firstNewline < 0 || string(bytes.TrimSuffix(raw[:firstNewline], []byte{'\r'})) != "---" {
		return nil, fmt.Errorf("load agent skill: SKILL.md must start with YAML frontmatter")
	}

	start := firstNewline + 1
	offset := start
	for offset <= len(raw) {
		relativeEnd := bytes.IndexByte(raw[offset:], '\n')
		end := len(raw)
		if relativeEnd >= 0 {
			end = offset + relativeEnd
		}
		line := bytes.TrimSuffix(raw[offset:end], []byte{'\r'})
		if string(line) == "---" {
			return raw[start:offset], nil
		}
		if relativeEnd < 0 {
			break
		}
		offset = end + 1
	}
	return nil, fmt.Errorf("load agent skill: SKILL.md frontmatter is not terminated")
}

func parseFrontmatter(raw []byte) (frontmatter, error) {
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return frontmatter{}, fmt.Errorf("load agent skill: parse SKILL.md frontmatter: %w", err)
	}

	name, err := requiredString(document, "name")
	if err != nil {
		return frontmatter{}, err
	}
	description, err := requiredString(document, "description")
	if err != nil {
		return frontmatter{}, err
	}
	license, err := optionalString(document, "license")
	if err != nil {
		return frontmatter{}, err
	}
	compatibility, err := optionalString(document, "compatibility")
	if err != nil {
		return frontmatter{}, err
	}
	allowedTools, err := optionalString(document, "allowed-tools")
	if err != nil {
		return frontmatter{}, err
	}
	metadata, err := metadataValues(document["metadata"])
	if err != nil {
		return frontmatter{}, err
	}
	return frontmatter{
		Name:          name,
		Description:   description,
		License:       license,
		Compatibility: compatibility,
		Metadata:      metadata,
		AllowedTools:  allowedTools,
	}, nil
}

func requiredString(document map[string]any, key string) (string, error) {
	value, exists := document[key]
	if !exists {
		return "", fmt.Errorf("load agent skill: frontmatter %s is required", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("load agent skill: frontmatter %s must be a string", key)
	}
	return text, nil
}

func optionalString(document map[string]any, key string) (string, error) {
	value, exists := document[key]
	if !exists || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("load agent skill: frontmatter %s must be a string", key)
	}
	return text, nil
}

func metadataValues(raw any) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("load agent skill: frontmatter metadata must be a mapping")
	}
	result := make(map[string]string, len(values))
	for key, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf(
				"load agent skill: metadata value for %q must be a string",
				key,
			)
		}
		result[key] = value
	}
	return result, nil
}

func validateMetadata(
	directoryName string,
	parsed frontmatter,
	limits Limits,
) (skilldomain.Metadata, error) {
	if parsed.Name == "" {
		return skilldomain.Metadata{}, fmt.Errorf("load agent skill: frontmatter name is required")
	}
	if len(parsed.Name) > 64 || !skillNamePattern.MatchString(parsed.Name) {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: name must be 1-64 lowercase letters, numbers, or hyphen-separated segments",
		)
	}
	if parsed.Name != directoryName {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: name %q must match directory %q",
			parsed.Name,
			directoryName,
		)
	}
	if strings.TrimSpace(parsed.Description) == "" {
		return skilldomain.Metadata{}, fmt.Errorf("load agent skill: description is required")
	}
	if utf8.RuneCountInString(parsed.Description) > 1024 {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: description must be at most 1024 characters",
		)
	}
	if parsed.Compatibility != "" &&
		utf8.RuneCountInString(parsed.Compatibility) > 500 {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: compatibility must be at most 500 characters",
		)
	}
	if utf8.RuneCountInString(parsed.License) > limits.MaxLicenseRunes {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: license exceeds %d characters",
			limits.MaxLicenseRunes,
		)
	}
	if utf8.RuneCountInString(parsed.AllowedTools) > limits.MaxAllowedToolsRunes {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: allowed-tools exceeds %d characters",
			limits.MaxAllowedToolsRunes,
		)
	}
	if len(parsed.Metadata) > limits.MaxMetadataEntries {
		return skilldomain.Metadata{}, fmt.Errorf(
			"load agent skill: metadata contains more than %d entries",
			limits.MaxMetadataEntries,
		)
	}
	values := make(map[string]string, len(parsed.Metadata))
	for key, value := range parsed.Metadata {
		if strings.TrimSpace(key) == "" {
			return skilldomain.Metadata{}, fmt.Errorf("load agent skill: metadata key must not be empty")
		}
		if utf8.RuneCountInString(key) > limits.MaxMetadataKeyRunes {
			return skilldomain.Metadata{}, fmt.Errorf(
				"load agent skill: metadata key exceeds %d characters",
				limits.MaxMetadataKeyRunes,
			)
		}
		if utf8.RuneCountInString(value) > limits.MaxMetadataValueRunes {
			return skilldomain.Metadata{}, fmt.Errorf(
				"load agent skill: metadata value for %q exceeds %d characters",
				key,
				limits.MaxMetadataValueRunes,
			)
		}
		values[key] = value
	}
	return skilldomain.Metadata{
		Name:          parsed.Name,
		Description:   parsed.Description,
		License:       parsed.License,
		Compatibility: parsed.Compatibility,
		Values:        values,
		AllowedTools:  parsed.AllowedTools,
	}, nil
}

func inventoryFSResources(
	source fs.FS,
	root string,
	limits Limits,
) ([]skilldomain.Resource, int64, error) {
	resources := make([]skilldomain.Resource, 0)
	var totalBytes int64

	err := fs.WalkDir(source, root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", fullPath, walkErr)
		}
		if fullPath == root {
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("root must not be a symlink")
			}
			if !entry.IsDir() {
				return fmt.Errorf("root must be a directory")
			}
			return nil
		}
		relativePath := strings.TrimPrefix(fullPath, root+"/")
		if relativePath == fullPath || relativePath == "" {
			return fmt.Errorf("resource %q is outside skill root", fullPath)
		}
		if len(relativePath) > limits.MaxPathBytes {
			return fmt.Errorf("resource path %q exceeds %d bytes", relativePath, limits.MaxPathBytes)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("resource %q must not be a symlink", relativePath)
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("resource %q info: %w", relativePath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("resource %q must be a regular file", relativePath)
		}
		if len(resources)+1 > limits.MaxFiles {
			return fmt.Errorf("skill contains more than %d files", limits.MaxFiles)
		}
		if info.Size() > limits.MaxFileBytes {
			return fmt.Errorf("resource %q exceeds %d bytes", relativePath, limits.MaxFileBytes)
		}
		if totalBytes > limits.MaxTotalBytes-info.Size() {
			return fmt.Errorf("skill exceeds %d total bytes", limits.MaxTotalBytes)
		}

		raw, err := readFSResource(source, fullPath, limits.MaxFileBytes)
		if err != nil {
			return fmt.Errorf("read resource %q: %w", relativePath, err)
		}
		sum := sha256.Sum256(raw)
		resources = append(resources, skilldomain.Resource{
			Path:       relativePath,
			Kind:       classifyResource(relativePath),
			Size:       info.Size(),
			SHA256:     "sha256:" + hex.EncodeToString(sum[:]),
			Executable: info.Mode().Perm()&0o111 != 0,
		})
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("load agent skill: %w", err)
	}
	return normalizeResources(resources, totalBytes)
}

func inventoryResources(
	root string,
	limits Limits,
) ([]skilldomain.Resource, int64, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, 0, fmt.Errorf("load agent skill: open root: %w", err)
	}
	defer func() { _ = rootHandle.Close() }()

	resources := make([]skilldomain.Resource, 0)
	var totalBytes int64

	err = fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if path == "." {
			return nil
		}
		if len(path) > limits.MaxPathBytes {
			return fmt.Errorf("resource path %q exceeds %d bytes", path, limits.MaxPathBytes)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("resource %q must not be a symlink", path)
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("resource %q info: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("resource %q must be a regular file", path)
		}
		if len(resources)+1 > limits.MaxFiles {
			return fmt.Errorf("skill contains more than %d files", limits.MaxFiles)
		}
		if info.Size() > limits.MaxFileBytes {
			return fmt.Errorf("resource %q exceeds %d bytes", path, limits.MaxFileBytes)
		}
		if totalBytes > limits.MaxTotalBytes-info.Size() {
			return fmt.Errorf("skill exceeds %d total bytes", limits.MaxTotalBytes)
		}

		file, err := rootHandle.Open(filepath.FromSlash(path))
		if err != nil {
			return fmt.Errorf("open resource %q: %w", path, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read resource %q: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close resource %q: %w", path, closeErr)
		}
		if int64(len(raw)) != info.Size() {
			return fmt.Errorf("resource %q changed while being inventoried", path)
		}
		sum := sha256.Sum256(raw)
		resources = append(resources, skilldomain.Resource{
			Path:       path,
			Kind:       classifyResource(path),
			Size:       info.Size(),
			SHA256:     "sha256:" + hex.EncodeToString(sum[:]),
			Executable: info.Mode().Perm()&0o111 != 0,
		})
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("load agent skill: %w", err)
	}

	return normalizeResources(resources, totalBytes)
}

func normalizeResources(
	resources []skilldomain.Resource,
	totalBytes int64,
) ([]skilldomain.Resource, int64, error) {
	slices.SortFunc(resources, func(a, b skilldomain.Resource) int {
		return strings.Compare(a.Path, b.Path)
	})
	if !slices.ContainsFunc(resources, func(resource skilldomain.Resource) bool {
		return resource.Path == "SKILL.md"
	}) {
		return nil, 0, fmt.Errorf("load agent skill: SKILL.md is required")
	}
	return resources, totalBytes, nil
}

func classifyResource(path string) skilldomain.ResourceKind {
	if path == "SKILL.md" {
		return skilldomain.ResourceManifest
	}
	first, _, _ := strings.Cut(path, "/")
	switch first {
	case "scripts":
		return skilldomain.ResourceScript
	case "references":
		return skilldomain.ResourceReference
	case "assets":
		return skilldomain.ResourceAsset
	default:
		return skilldomain.ResourceOther
	}
}

func snapshotDigest(resources []skilldomain.Resource) string {
	hash := sha256.New()
	var encoded [8]byte
	for _, resource := range resources {
		writeDigestField(hash, encoded[:], resource.Path)
		writeDigestField(hash, encoded[:], string(resource.Kind))
		writeDigestField(hash, encoded[:], resource.SHA256)

		writeDigestField(hash, encoded[:], strconv.FormatInt(resource.Size, 10))
		if resource.Executable {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeDigestField(writer byteWriter, encoded []byte, value string) {
	binary.BigEndian.PutUint64(encoded, uint64(len(value)))
	_, _ = writer.Write(encoded)
	_, _ = writer.Write([]byte(value))
}
