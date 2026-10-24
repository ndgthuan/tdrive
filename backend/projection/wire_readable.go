package projection

import (
	"strconv"
	"strings"
	"time"
)

// Readable wire format.
//
// New writes look like
//
//	TDrive | type=file | parent=root | name=personal.jpg | size=382251 | time=2026-10-23T16:22:05+00:00
//
// Every field is "key=value", fields are joined by " | ", and every key is a
// full word. Values keep the legacy percent-encoding (url.QueryEscape for
// names, hashes, hints and JSON), so a name containing " | " or "=" is stored
// as escaped text and cannot split a field. Timestamps are ISO-8601 in UTC
// with an explicit "+00:00" offset.
//
// The legacy "TDX1|t=f|p=|n=..." form is still accepted on read. Both forms
// decode through the same key/value table into the same Op, so the legacy
// tail of Parse is shared by old and new headers.

const (
	readable_wire_prefix    = "TDrive"
	readable_wire_separator = " | "
	readable_root_word      = "root"
	readable_time_layout    = "2006-01-02T15:04:05-07:00"
)

// readable_key_pairs maps every legacy key the writer emits to its full-word
// name. Each legacy key has exactly one canonical readable name.
var readable_key_pairs = [][2]string{
	{"t", "type"},
	{"p", "parent"},
	{"n", "name"},
	{"obj", "object"},
	{"sz", "size"},
	{"ts", "time"},
	{"enc", "encrypted"},
	{"ev", "encryption_version"},
	{"psz", "plain_size"},
	{"u", "upload_uuid"},
	{"pix", "part_index"},
	{"pc", "part_count"},
	{"salt", "kdf_salt"},
	{"kdf", "kdf_params"},
	{"wrap", "wrapped_master_key"},
	{"check", "key_check"},
	{"hint", "hint"},
	{"cv", "config_version"},
	{"v", "version"},
	{"oid", "operation_id"},
	{"cmid", "content_message_id"},
	{"hash", "content_hash"},
	{"rev", "revision"},
	{"ow", "overwrite"},
	{"dst", "destination_object"},
	{"drev", "destination_revision"},
	{"del", "deleted_at"},
	{"purge", "purge_at"},
	{"retain", "retain_until"},
	{"rend", "rendition"},
}

// readable_type_pairs maps each legacy op type code to its readable name.
var readable_type_pairs = [][2]string{
	{"f", "file"},
	{"mkdir", "make_directory"},
	{"rename", "rename"},
	{"move", "move"},
	{"rmdir", "remove_directory"},
	{"tomb", "tombstone"},
	{"meta", "metadata"},
	{"encfg", "encryption_config"},
	{"part", "file_part"},
	{"manifest", "file_manifest"},
	{"fcommit", "file_commit"},
	{"freplace", "file_replace"},
	{"dcommit", "folder_commit"},
	{"relocate", "relocate"},
	{"trash", "trash"},
	{"harddel", "hard_delete"},
	{"untrash", "restore"},
}

// readable_timestamp_keys are the legacy keys whose value is unix seconds and
// whose readable form is ISO-8601 UTC.
var readable_timestamp_keys = map[string]bool{
	"ts":     true,
	"del":    true,
	"purge":  true,
	"retain": true,
}

var (
	readable_name_by_legacy_key = build_pair_index(readable_key_pairs, 0)
	legacy_key_by_readable_name = build_pair_index(readable_key_pairs, 1)
	readable_name_by_type       = build_pair_index(readable_type_pairs, 0)
	legacy_type_by_readable     = build_pair_index(readable_type_pairs, 1)
)

func build_pair_index(pairs [][2]string, from int) map[string]string {
	index := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		index[pair[from]] = pair[1-from]
	}
	return index
}

// IsManagedHeader reports whether a caption's first line claims to be a TDrive
// header in either wire format. Sync and attachment handling use it so that a
// malformed TDrive header is never mistaken for an ordinary captionless file.
func IsManagedHeader(header string) bool {
	header = strings.TrimSpace(header)
	return strings.HasPrefix(header, wireVersionPrefix) ||
		header == readable_wire_prefix ||
		strings.HasPrefix(header, readable_wire_prefix+readable_wire_separator)
}

// format_iso_utc renders unix seconds as ISO-8601 UTC with an explicit
// "+00:00" offset (never "Z"), e.g. 2018-02-13T23:18:39+00:00.
func format_iso_utc(unix_seconds int64) string {
	return time.Unix(unix_seconds, 0).UTC().Format(readable_time_layout)
}

// parse_iso_utc decodes an ISO-8601 timestamp with an offset into unix seconds.
func parse_iso_utc(value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, ErrWireMalformed
	}
	return t.Unix(), nil
}

// parse_wire_fields returns the legacy-keyed field map for either wire form.
// Callers see one vocabulary whichever prefix the header used.
func parse_wire_fields(header string) (map[string]string, error) {
	if header == readable_wire_prefix || strings.HasPrefix(header, readable_wire_prefix+readable_wire_separator) {
		return parse_readable_fields(header)
	}
	return parse_legacy_fields(header)
}

func parse_legacy_fields(header string) (map[string]string, error) {
	parts := strings.Split(header, "|")
	if len(parts) < 2 {
		return nil, ErrWireMalformed
	}
	if parts[0] != wireVersionPrefix {
		return nil, ErrWireBadVersion
	}
	kv := make(map[string]string, len(parts)-1)
	for _, p := range parts[1:] {
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			return nil, ErrWireMalformed
		}
		kv[p[:eq]] = p[eq+1:]
	}
	return kv, nil
}

func parse_readable_fields(header string) (map[string]string, error) {
	fields := strings.Split(header, readable_wire_separator)
	if len(fields) < 2 {
		return nil, ErrWireMalformed
	}
	kv := make(map[string]string, len(fields)-1)
	for _, field := range fields[1:] {
		eq := strings.IndexByte(field, '=')
		if eq <= 0 {
			return nil, ErrWireMalformed
		}
		name, value := field[:eq], field[eq+1:]
		legacy, known := legacy_key_by_readable_name[name]
		if !known {
			// Unknown full-word keys come from newer writers and are ignored,
			// exactly like unknown legacy keys.
			continue
		}
		switch {
		case legacy == "t":
			code, ok := legacy_type_by_readable[value]
			if !ok {
				return nil, ErrWireBadOpType
			}
			value = code
		case legacy == "p":
			if value == readable_root_word {
				value = ""
			}
		case readable_timestamp_keys[legacy]:
			unix, err := parse_iso_utc(value)
			if err != nil {
				return nil, err
			}
			value = strconv.FormatInt(unix, 10)
		}
		kv[legacy] = value
	}
	return kv, nil
}

// format_readable converts one legacy-form header into the readable form. The
// legacy writer escapes every free-text value, so splitting it on "|" is exact.
func format_readable(legacy string) string {
	legacy_fields := strings.Split(legacy, "|")
	out := make([]string, 0, len(legacy_fields))
	out = append(out, readable_wire_prefix)
	for _, field := range legacy_fields[1:] {
		key, value, _ := strings.Cut(field, "=")
		name, known := readable_name_by_legacy_key[key]
		if !known {
			name = key
		}
		switch {
		case key == "t":
			if readable, ok := readable_name_by_type[value]; ok {
				value = readable
			}
		case key == "p" && value == "":
			value = readable_root_word
		case readable_timestamp_keys[key]:
			if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
				value = format_iso_utc(unix)
			}
		}
		out = append(out, name+"="+value)
	}
	return strings.Join(out, readable_wire_separator)
}
