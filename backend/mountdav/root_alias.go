package mountdav

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// rootAliasKind classifies a cleaned, capability-relative WebDAV path when a
// root alias is configured. Without an alias every path is a drive path.
type rootAliasKind int

const (
	// rootAliasDrive is a path that maps to the same drive path (no alias).
	rootAliasDrive rootAliasKind = iota
	// rootAliasCapabilityRoot is the virtual capability root "/". With an alias
	// it holds exactly one child, the alias folder, and nothing is writable.
	rootAliasCapabilityRoot
	// rootAliasFolder is the alias folder itself. It reads as the drive root,
	// but it cannot be created, renamed, moved, deleted, or locked.
	rootAliasFolder
	// rootAliasInside is a path below the alias folder. It maps to the drive.
	rootAliasInside
	// rootAliasOutside is any other path under the capability. With an alias
	// configured it does not exist for reads and is refused for writes.
	rootAliasOutside
)

// resolveRootAlias maps a cleaned capability-relative path to the drive path
// that backs it. The capability root maps to the drive root for metadata only;
// its listing is synthesized by FileSystem.childEntries.
func resolveRootAlias(alias, clean string) (kind rootAliasKind, drive string) {
	if alias == "" {
		return rootAliasDrive, clean
	}
	switch {
	case clean == "/":
		return rootAliasCapabilityRoot, "/"
	case clean == "/"+alias:
		return rootAliasFolder, "/"
	case strings.HasPrefix(clean, "/"+alias+"/"):
		return rootAliasInside, "/" + strings.TrimPrefix(clean, "/"+alias+"/")
	default:
		return rootAliasOutside, "/"
	}
}

// validateRootAlias accepts an empty alias (disabled) or one plain folder name.
func validateRootAlias(alias string) error {
	if alias == "" {
		return nil
	}
	if !utf8.ValidString(alias) || strings.TrimSpace(alias) != alias ||
		alias == "." || alias == ".." || strings.ContainsAny(alias, "/\\\x00") ||
		len(alias) > maxWritableComponentBytes {
		return fmt.Errorf("mountdav: invalid root alias")
	}
	return nil
}

// driveConditions returns conditions whose resource paths are mapped from
// capability-relative paths to drive paths. The input is never modified.
func driveConditions(alias string, conditions MutationConditions) (MutationConditions, bool) {
	if alias == "" || len(conditions.DAVIf) == 0 {
		return conditions, true
	}
	next := conditions
	next.DAVIf = make([]DAVConditionList, len(conditions.DAVIf))
	for index, list := range conditions.DAVIf {
		kind, drive := resolveRootAlias(alias, list.ResourcePath)
		if kind != rootAliasInside && kind != rootAliasFolder {
			return MutationConditions{}, false
		}
		next.DAVIf[index] = DAVConditionList{ResourcePath: drive, Conditions: list.Conditions}
	}
	return next, true
}

// aliasedWriter translates capability-relative paths and condition resources
// into drive paths before they reach the real coordinator. Paths outside the
// alias folder are refused here as a second line of defense; requestResourcePath
// refuses them earlier with a precise status.
type aliasedWriter struct {
	alias string
	next  WriteCoordinator
}

func newAliasedWriter(alias string, next WriteCoordinator) WriteCoordinator {
	if alias == "" || next == nil {
		return next
	}
	return &aliasedWriter{alias: alias, next: next}
}

func (writer *aliasedWriter) drivePath(path string) (string, error) {
	kind, drive := resolveRootAlias(writer.alias, path)
	if kind != rootAliasInside {
		return "", ErrWriteForbidden
	}
	return drive, nil
}

func (writer *aliasedWriter) Put(ctx context.Context, request PutRequest, body io.Reader) (MutationResult, error) {
	path, err := writer.drivePath(request.Path)
	if err != nil {
		return MutationResult{}, err
	}
	conditions, ok := driveConditions(writer.alias, request.Conditions)
	if !ok {
		return MutationResult{}, ErrWriteForbidden
	}
	request.Path = path
	request.Conditions = conditions
	return writer.next.Put(ctx, request, body)
}

func (writer *aliasedWriter) Mkdir(ctx context.Context, request MkdirRequest) (MutationResult, error) {
	path, err := writer.drivePath(request.Path)
	if err != nil {
		return MutationResult{}, err
	}
	conditions, ok := driveConditions(writer.alias, request.Conditions)
	if !ok {
		return MutationResult{}, ErrWriteForbidden
	}
	request.Path = path
	request.Conditions = conditions
	return writer.next.Mkdir(ctx, request)
}

func (writer *aliasedWriter) Move(ctx context.Context, request MoveRequest) (MutationResult, error) {
	source, err := writer.drivePath(request.SourcePath)
	if err != nil {
		return MutationResult{}, err
	}
	destination, err := writer.drivePath(request.DestinationPath)
	if err != nil {
		return MutationResult{}, err
	}
	conditions, ok := driveConditions(writer.alias, request.Conditions)
	if !ok {
		return MutationResult{}, ErrWriteForbidden
	}
	request.SourcePath = source
	request.DestinationPath = destination
	request.Conditions = conditions
	return writer.next.Move(ctx, request)
}

func (writer *aliasedWriter) Delete(ctx context.Context, request DeleteRequest) (MutationResult, error) {
	path, err := writer.drivePath(request.Path)
	if err != nil {
		return MutationResult{}, err
	}
	conditions, ok := driveConditions(writer.alias, request.Conditions)
	if !ok {
		return MutationResult{}, ErrWriteForbidden
	}
	request.Path = path
	request.Conditions = conditions
	return writer.next.Delete(ctx, request)
}

var _ WriteCoordinator = (*aliasedWriter)(nil)
