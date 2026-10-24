package mountos

import (
	"errors"
	"strings"
)

var err_invalid_mount_remote = errors.New("mount OS: invalid Windows mount remote")

// build_label_key converts a Windows UNC remote such as
// \\127.0.0.1@54336\DavWWWRoot\tdrive-<hex> into the MountPoints2 subkey name
// Explorer uses, ##127.0.0.1@54336#DavWWWRoot#tdrive-<hex>. Backslashes become
// '#'. A '#' already present in the remote is rejected rather than escaped,
// because it would make the mapping from key name back to remote ambiguous.
// This is pure string logic with no registry dependency.
//
//lint:ignore U1000 used by connector_windows.go and label_windows.go only, behind //go:build windows.
func build_label_key(remote string) (string, error) {
	trimmed := strings.TrimSuffix(remote, `\`)
	if !strings.HasPrefix(trimmed, `\\`) || strings.Contains(trimmed, "#") {
		return "", err_invalid_mount_remote
	}
	segments := strings.Split(trimmed[2:], `\`)
	if len(segments) < 2 {
		return "", err_invalid_mount_remote
	}
	for _, segment := range segments {
		if segment == "" || strings.ContainsAny(segment, "/:") {
			return "", err_invalid_mount_remote
		}
	}
	return "##" + strings.Join(segments, "#"), nil
}

const dav_www_root_segment = "DavWWWRoot"

// build_label_key_variants returns the MountPoints2 subkey names that describe
// remote. The first entry is build_label_key(remote). The second toggles the
// DavWWWRoot segment: it is removed when remote has one, and inserted after the
// host when it has none. Explorer has been observed to read the key without
// that segment, so the label is written to both spellings. A remote that is
// exactly \host\DavWWWRoot has no usable variant and yields only the primary.
// This is pure string logic with no registry dependency.
//
//lint:ignore U1000 used by label_windows.go only, behind //go:build windows.
func build_label_key_variants(remote string) ([]string, error) {
	primary, err := build_label_key(remote)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(primary[2:], "#")
	has_root := strings.EqualFold(segments[1], dav_www_root_segment)
	var toggled []string
	switch {
	case has_root && len(segments) >= 3:
		toggled = append([]string{segments[0]}, segments[2:]...)
	case has_root:
		return []string{primary}, nil
	default:
		toggled = append([]string{segments[0], dav_www_root_segment}, segments[1:]...)
	}
	return []string{primary, "##" + strings.Join(toggled, "#")}, nil
}
