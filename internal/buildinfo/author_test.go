// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package buildinfo

import (
	"strings"
	"testing"
)

func TestAttributionConstantsAreNonEmpty(t *testing.T) {
	for name, value := range map[string]string{
		"Author":      Author,
		"Repository":  Repository,
		"Upstream":    Upstream,
		"UpstreamURL": UpstreamURL,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s is empty", name)
		}
	}
}

func TestAttributionNamesAuthorAndUpstream(t *testing.T) {
	line := Attribution()
	for _, want := range []string{Author, Upstream} {
		if !strings.Contains(line, want) {
			t.Fatalf("Attribution() = %q, missing %q", line, want)
		}
	}
}
