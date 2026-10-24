package projection

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// every_op_type_fixture returns one fully populated op for each wire op type.
// Each one exercises the fields that its legacy and readable forms carry.
func every_op_type_fixture() []Op {
	return []Op{
		{Type: OpFileUpload, Parent: RootParent, Name: "personal.jpg", FileSize: 382251, FileUploadTime: 1792772525},
		{Type: OpFileUpload, Parent: "d:abc", Name: "Goa Trip | 2026 = notes.txt", FileSize: 12, FileUploadTime: 1518563919,
			Encrypted: true, EncryptionVersion: 1, PlaintextSize: 10},
		{Type: OpMeta, Obj: "f:9", Parent: "d:abc", Name: "renamed.bin", FileSize: 2048, FileUploadTime: 88},
		{Type: OpMkdir, Obj: "d:a", Parent: RootParent, Name: "A"},
		{Type: OpRename, Obj: "f:42", Name: "new.png"},
		{Type: OpRename, Obj: "d:abc", Name: "Goa%20Trip"},
		{Type: OpMove, Obj: "f:7", Parent: "d:abc"},
		{Type: OpMove, Obj: "d:abc", Parent: RootParent},
		{Type: OpRmdir, Obj: "d:gone"},
		{Type: OpTomb, Obj: "f:99"},
		{Type: OpFilePart, UploadUUID: "upload-1", PartIndex: 2, FileSize: 4096},
		{Type: OpFileManifest, UploadUUID: "upload-1", Parent: RootParent, Name: "big.zip", PartCount: 3, FileSize: 9000, FileUploadTime: 1000},
		{Type: OpEncConfig, KDFSalt: []byte{1, 2, 3}, KDFParamsJSON: `{"m":1}`, WrappedMasterKey: []byte{4, 5},
			KeyCheck: []byte{6}, Hint: "my hint | x=y", ConfigVersion: 2},
		{Type: OpFileCommit, ProtocolVersion: 1, OpID: "commit-1", Parent: "d:docs", Name: "file.txt",
			ContentMsgID: 91, ContentHash: "sha256:abc", FileSize: 12, FileUploadTime: 34},
		{Type: OpFileReplace, ProtocolVersion: 1, OpID: "replace-1", Obj: "f:41", ExpectedRevision: 2,
			ContentMsgID: 92, ContentHash: "sha256:def", FileSize: 56, FileUploadTime: 78, RetainedUntil: 200},
		{Type: OpFolderCommit, ProtocolVersion: 1, OpID: "mkdir-1", Obj: "d:docs", Parent: RootParent, Name: "Docs"},
		{Type: OpRelocate, ProtocolVersion: 1, OpID: "move-1", Obj: "f:41", Parent: "d:docs", Name: "renamed.txt",
			ExpectedRevision: 3, Overwrite: true, DestinationObj: "f:42", ExpectedDestinationRevision: 5,
			DeletedAt: 100, PurgeAfter: 300},
		{Type: OpTrashTree, ProtocolVersion: 1, OpID: "trash-1", Obj: "d:docs", ExpectedRevision: 4,
			DeletedAt: 1792773987, PurgeAfter: 1795365987},
		{Type: OpHardDeleteTree, ProtocolVersion: 1, OpID: "purge-440d", Obj: "f:4", ExpectedRevision: 2},
		{Type: OpRestoreTree, ProtocolVersion: 1, OpID: "restore-1", Obj: "f:5", Parent: RootParent, Name: "back.txt",
			ExpectedRevision: 1},
	}
}

func TestReadableEveryOpTypeRoundTrips(t *testing.T) {
	for _, want := range every_op_type_fixture() {
		raw := Format(want)
		if !strings.HasPrefix(raw, "TDrive | type=") {
			t.Fatalf("Format(%s) = %q, missing readable prefix", want.Type, raw)
		}
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip %s\n got: %#v\nwant: %#v\nwire: %s", want.Type, got, want, raw)
		}
	}
}

// Old and new headers must decode to identical Op values. The legacy form is
// produced by the retained legacy writer, so this compares the two generations
// of the same operation directly.
func TestReadableAndLegacyDecodeToSameOp(t *testing.T) {
	for _, op := range every_op_type_fixture() {
		legacy := format_legacy(op)
		readable := Format(op)
		if !strings.HasPrefix(legacy, "TDX1|") {
			t.Fatalf("legacy form %q lost its prefix", legacy)
		}
		fromLegacy, err := Parse(legacy)
		if err != nil {
			t.Fatalf("legacy Parse(%q): %v", legacy, err)
		}
		fromReadable, err := Parse(readable)
		if err != nil {
			t.Fatalf("readable Parse(%q): %v", readable, err)
		}
		if !reflect.DeepEqual(fromLegacy, fromReadable) {
			t.Fatalf("legacy and readable differ for %s\nlegacy:   %#v\nreadable: %#v", op.Type, fromLegacy, fromReadable)
		}
	}
}

func TestLegacyHeadersStillParseToKnownOps(t *testing.T) {
	cases := []struct {
		legacy   string
		readable string
	}{
		{
			legacy:   "TDX1|t=f|p=d:abc|n=sunset.jpg|sz=4321|ts=99\nTDrive: sunset.jpg",
			readable: "TDrive | type=file | parent=d:abc | name=sunset.jpg | size=4321 | time=" + format_iso_utc(99) + "\nTDrive: sunset.jpg",
		},
		{
			legacy:   "TDX1|t=mkdir|obj=d:goa-uuid|p=|n=Goa%20Trip",
			readable: "TDrive | type=make_directory | object=d:goa-uuid | parent=root | name=Goa%20Trip",
		},
		{
			legacy:   "TDX1|t=trash|v=1|oid=trash-f6ca|obj=f:5|rev=1|del=1792773987|purge=1795365987",
			readable: "TDrive | type=trash | version=1 | operation_id=trash-f6ca | object=f:5 | revision=1 | deleted_at=" + format_iso_utc(1792773987) + " | purge_at=" + format_iso_utc(1795365987),
		},
		{
			legacy:   "TDX1|t=harddel|v=1|oid=purge-440d|obj=f:4|rev=2",
			readable: "TDrive | type=hard_delete | version=1 | operation_id=purge-440d | object=f:4 | revision=2",
		},
		{
			legacy:   "TDX1|t=tomb|obj=f:2",
			readable: "TDrive | type=tombstone | object=f:2",
		},
		{
			legacy:   "TDX1|t=f|p=|n=Goa+Trip%7Cx%3Dy|sz=1",
			readable: "TDrive | type=file | parent=root | name=Goa+Trip%7Cx%3Dy | size=1",
		},
	}
	for _, c := range cases {
		want, err := Parse(c.legacy)
		if err != nil {
			t.Fatalf("legacy Parse(%q): %v", c.legacy, err)
		}
		got, err := Parse(c.readable)
		if err != nil {
			t.Fatalf("readable Parse(%q): %v", c.readable, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("readable %q\n got: %#v\nwant: %#v", c.readable, got, want)
		}
	}
}

func TestFormatTimestampsAreISOWithExplicitUTCOffset(t *testing.T) {
	unix := time.Date(2018, 2, 13, 23, 18, 39, 0, time.UTC).Unix()
	if got := format_iso_utc(unix); got != "2018-02-13T23:18:39+00:00" {
		t.Fatalf("format_iso_utc = %q", got)
	}
	raw := Format(Op{Type: OpTrashTree, ProtocolVersion: 1, OpID: "t", Obj: "f:5", ExpectedRevision: 1,
		DeletedAt: unix, PurgeAfter: unix + 86400})
	for _, want := range []string{
		" | deleted_at=2018-02-13T23:18:39+00:00",
		" | purge_at=2018-02-14T23:18:39+00:00",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("wire %q missing %q", raw, want)
		}
	}
	if strings.Contains(raw, "Z |") || strings.HasSuffix(raw, "Z") {
		t.Fatalf("wire %q used Z instead of +00:00", raw)
	}
	upload := Format(Op{Type: OpFileUpload, Parent: RootParent, Name: "x", FileSize: 1, FileUploadTime: unix})
	if !strings.Contains(upload, " | time=2018-02-13T23:18:39+00:00") {
		t.Fatalf("upload wire %q", upload)
	}
}

func TestReadableTimestampRoundTripsExactly(t *testing.T) {
	want := Op{Type: OpFileUpload, Parent: RootParent, Name: "x", FileSize: 1, FileUploadTime: 1518563919}
	got, err := Parse(Format(want))
	if err != nil || got.FileUploadTime != want.FileUploadTime {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := Parse("TDrive | type=file | parent=root | name=x | time=2018-02-13 23:18:39"); !errors.Is(err, ErrWireMalformed) {
		t.Fatalf("non-ISO time err = %v, want ErrWireMalformed", err)
	}
}

func TestReadableNamesWithSeparatorsRoundTrip(t *testing.T) {
	names := []string{
		"a | b.txt",
		"key=value.txt",
		"pipe|only",
		"equals=and | pipe=.txt",
		"100% sure + more.pdf",
		"tabs\tand spaces.doc",
	}
	for _, name := range names {
		for _, op := range []Op{
			{Type: OpFileUpload, Parent: RootParent, Name: name, FileSize: 3},
			{Type: OpMkdir, Obj: "d:x", Parent: "d:p", Name: name},
			{Type: OpRename, Obj: "f:8", Name: name},
		} {
			raw := Format(op)
			if strings.Count(raw, " | ") < 2 {
				t.Fatalf("separator count broken for %q: %q", name, raw)
			}
			got, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse(%q): %v", raw, err)
			}
			if got.Name != name {
				t.Fatalf("name %q came back as %q (wire %q)", name, got.Name, raw)
			}
		}
	}
}

func TestReadableHintWithSeparatorsRoundTrips(t *testing.T) {
	op := Op{Type: OpEncConfig, KDFSalt: []byte{1}, KDFParamsJSON: `{"a":"b|c"}`, WrappedMasterKey: []byte{2},
		KeyCheck: []byte{3}, Hint: "hint | with = signs", ConfigVersion: 1}
	got, err := Parse(Format(op))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hint != op.Hint || got.KDFParamsJSON != op.KDFParamsJSON {
		t.Fatalf("hint or params lost: %q %q", got.Hint, got.KDFParamsJSON)
	}
}

func TestReadableRootParentWords(t *testing.T) {
	got, err := Parse("TDrive | type=file | parent=root | name=x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Parent != RootParent {
		t.Fatalf("parent = %q, want root", got.Parent)
	}
}

func TestReadableRejectsMalformedAndUnknownInput(t *testing.T) {
	cases := []struct {
		header string
		want   error
	}{
		{"TDrive", ErrWireMalformed},
		{"TDrive | type=file | name", ErrWireMalformed},
		{"TDrive | type=file | =x", ErrWireMalformed},
		{"TDrive | type=file | name=x | size=-1", ErrWireMalformed},
		{"TDrive | type=file | name=x | time=-1", ErrWireMalformed},
		{"TDrive | type=file | name=x | time=bogus", ErrWireMalformed},
		{"TDrive | type=file | parent=bad | name=x", ErrWireBadParent},
		{"TDrive | type=quux", ErrWireBadOpType},
		{"TDrive | type=f | name=x", ErrWireBadOpType},
		{"TDrive | name=x", ErrWireMalformed},
		{"TDrive |type=file | name=x", ErrWireBadVersion},
		{"TDrive|type=file|name=x", ErrWireBadVersion},
		{"TDX2|t=f|n=x", ErrWireBadVersion},
		{"TDrive: sunset.jpg", ErrWireMalformed},
	}
	for _, c := range cases {
		if _, err := Parse(c.header); !errors.Is(err, c.want) {
			t.Fatalf("Parse(%q) err = %v, want %v", c.header, err, c.want)
		}
	}
}

func TestReadableUnknownKeysAreIgnored(t *testing.T) {
	got, err := Parse("TDrive | type=file | parent=root | name=x.txt | future_field=1 | size=5")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "x.txt" || got.FileSize != 5 {
		t.Fatalf("got %+v", got)
	}
}

func TestIsManagedHeaderRecognisesBothWireForms(t *testing.T) {
	for _, header := range []string{
		"TDX1|t=f|n=a",
		"  TDX1|t=f|n=a",
		"TDrive | type=file | name=a",
		"TDrive",
	} {
		if !IsManagedHeader(header) {
			t.Fatalf("IsManagedHeader(%q) = false", header)
		}
	}
	for _, header := range []string{
		"",
		"hello there",
		"TDrive: personal.jpg",
		"TDrive|type=file",
		"TDX2|t=f",
	} {
		if IsManagedHeader(header) {
			t.Fatalf("IsManagedHeader(%q) = true", header)
		}
	}
}

func TestReadableHeaderWithHumanCommentLine(t *testing.T) {
	raw := Format(Op{Type: OpFileUpload, Parent: RootParent, Name: "personal.jpg", FileSize: 1, FileUploadTime: 5}) +
		"\nTDrive: personal.jpg"
	if got := ExtractHeaderLine(raw); !IsManagedHeader(got) {
		t.Fatalf("header line %q not managed", got)
	}
	op, err := Parse(raw)
	if err != nil || op.Name != "personal.jpg" {
		t.Fatalf("Parse with comment: %+v, %v", op, err)
	}
}

func TestRawAttachmentCaptionAcceptsReadableUploadOnly(t *testing.T) {
	name, ok := RawAttachmentCaption("TDrive | type=file | parent=root | name=caption.pdf | size=12\nTDrive: caption.pdf", 12)
	if !ok || name != "caption.pdf" {
		t.Fatalf("readable upload: name=%q ok=%v", name, ok)
	}
	for _, text := range []string{
		"TDrive | type=file | name=caption.pdf | size=11",
		"TDrive | type=file | name=caption.pdf | upload_uuid=abc",
		"TDrive | type=file_part | upload_uuid=abc | part_index=0 | size=12",
		"TDrive | type=file | name=caption.pdf | encrypted=1 | plain_size=12 | encryption_version=1",
	} {
		if _, ok := RawAttachmentCaption(text, 12); ok {
			t.Fatalf("RawAttachmentCaption(%q) admitted a managed internal header", text)
		}
	}
}
