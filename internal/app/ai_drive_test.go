package app

import "testing"

func TestAIItemIDsMapToProjectionObjects(t *testing.T) {
	if got := aiItemObjectID("2615"); got != "f:2615" {
		t.Fatalf("file id should map to f: prefix, got %q", got)
	}
	if got := aiItemObjectID("d:abc"); got != "d:abc" {
		t.Fatalf("folder id should pass through, got %q", got)
	}
	if id, err := aiFileMsgID("2615"); err != nil || id != 2615 {
		t.Fatalf("file msg id: %d %v", id, err)
	}
	if _, err := aiFileMsgID("d:abc"); err == nil {
		t.Fatal("a folder id must not parse as a file id")
	}
	if _, err := aiFileMsgID("0"); err == nil {
		t.Fatal("zero is not a file id")
	}
}
