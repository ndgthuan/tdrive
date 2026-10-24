package mountos

import (
	"strings"
	"testing"
)

// unc turns a readable remote written with '|' separators into a Windows UNC
// string. Test inputs use '|' so the literals never depend on backslash escaping.
func unc(s string) string {
	return strings.ReplaceAll(s, "|", `\`)
}

func TestBuildLabelKeyMatchesExplorerSubkeyName(t *testing.T) {
	remote := unc(`||127.0.0.1@54336|DavWWWRoot|tdrive-0123456789abcdef`)
	want := `##127.0.0.1@54336#DavWWWRoot#tdrive-0123456789abcdef`
	for _, candidate := range []string{remote, remote + `\`} {
		got, err := build_label_key(candidate)
		if err != nil || got != want {
			t.Fatalf("build_label_key(%q) = %q, %v; want %q", candidate, got, err, want)
		}
	}
}

func TestBuildLabelKeyRejectsAmbiguousOrMalformedRemotes(t *testing.T) {
	for _, remote := range []string{
		"",
		unc(`127.0.0.1@54336|DavWWWRoot|tdrive-x`),
		unc(`||127.0.0.1@54336`),
		unc(`||127.0.0.1@54336||tdrive-x`),
		unc(`||127.0.0.1@54336|DavWWWRoot|tdrive#x`),
		unc(`||127.0.0.1@54336|DavWWWRoot|tdrive/x`),
		unc(`||127.0.0.1:54336|DavWWWRoot|tdrive-x`),
	} {
		if got, err := build_label_key(remote); err == nil {
			t.Fatalf("build_label_key(%q) = %q; want rejection", remote, got)
		}
	}
}

func TestBuildLabelKeyVariantsToggleDavWWWRootSegment(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   []string
	}{
		{
			name:   "remote without DavWWWRoot gains it",
			remote: unc(`||127.0.0.1@54336|tdrive-0123456789abcdef`),
			want: []string{
				`##127.0.0.1@54336#tdrive-0123456789abcdef`,
				`##127.0.0.1@54336#DavWWWRoot#tdrive-0123456789abcdef`,
			},
		},
		{
			name:   "remote with DavWWWRoot also yields the stripped key",
			remote: unc(`||127.0.0.1@54336|DavWWWRoot|tdrive-0123456789abcdef`),
			want: []string{
				`##127.0.0.1@54336#DavWWWRoot#tdrive-0123456789abcdef`,
				`##127.0.0.1@54336#tdrive-0123456789abcdef`,
			},
		},
		{
			name:   "trailing backslash is ignored",
			remote: unc(`||127.0.0.1@54336|tdrive-x`) + `\`,
			want: []string{
				`##127.0.0.1@54336#tdrive-x`,
				`##127.0.0.1@54336#DavWWWRoot#tdrive-x`,
			},
		},
		{
			name:   "DavWWWRoot match is case-insensitive",
			remote: unc(`||127.0.0.1@54336|davwwwroot|tdrive-x`),
			want: []string{
				`##127.0.0.1@54336#davwwwroot#tdrive-x`,
				`##127.0.0.1@54336#tdrive-x`,
			},
		},
		{
			name:   "bare DavWWWRoot share has no usable variant",
			remote: unc(`||127.0.0.1@54336|DavWWWRoot`),
			want:   []string{`##127.0.0.1@54336#DavWWWRoot`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := build_label_key_variants(test.remote)
			if err != nil {
				t.Fatalf("build_label_key_variants(%q) error = %v", test.remote, err)
			}
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("build_label_key_variants(%q) = %q; want %q", test.remote, got, test.want)
			}
		})
	}
}

func TestBuildLabelKeyVariantsRejectsMalformedRemotes(t *testing.T) {
	for _, remote := range []string{"", unc(`||127.0.0.1@54336`), unc(`||127.0.0.1@54336|tdrive#x`)} {
		if got, err := build_label_key_variants(remote); err == nil {
			t.Fatalf("build_label_key_variants(%q) = %q; want rejection", remote, got)
		}
	}
}
