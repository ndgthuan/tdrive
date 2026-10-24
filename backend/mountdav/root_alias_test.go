package mountdav

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/webdav"
)

const testRootAliasFolder = "Telegram Drive"

// newRootAliasTestHandler serves the test drive (Docs/note.txt) through the
// capability with alias set. It is writable through writer when writer is non-nil.
func newRootAliasTestHandler(t *testing.T, alias string, writer WriteCoordinator) http.Handler {
	t.Helper()
	isolate_data_dir(t)
	filesystem := testFS(t, nil)
	filesystem.alias = alias
	application := &readApplication{
		capabilityPath: testCapability,
		authority:      "127.0.0.1:7331",
		fs:             filesystem,
		lockSystem:     webdav.NewMemLS(),
		writer:         newAliasedWriter(alias, writer),
		rootAlias:      alias,
	}
	return newProtectedHandler(protectionConfig{
		capabilityPath:     testCapability,
		authority:          "127.0.0.1:7331",
		maxConcurrent:      4,
		maxConcurrentWrite: 4,
		writable:           writer != nil,
	}, application)
}

func aliasTarget(parts ...string) string {
	return testCapability + "/" + strings.Join(parts, "/")
}

func TestRootAliasResolvesPathsByKind(t *testing.T) {
	cases := []struct {
		path  string
		kind  rootAliasKind
		drive string
	}{
		{"/", rootAliasCapabilityRoot, "/"},
		{"/" + testRootAliasFolder, rootAliasFolder, "/"},
		{"/" + testRootAliasFolder + "/Docs/note.txt", rootAliasInside, "/Docs/note.txt"},
		{"/Telegram Drivex/Docs", rootAliasOutside, "/"},
		{"/Docs", rootAliasOutside, "/"},
	}
	for _, test := range cases {
		kind, drive := resolveRootAlias(testRootAliasFolder, test.path)
		if kind != test.kind || drive != test.drive {
			t.Fatalf("resolveRootAlias(%q) = %v, %q; want %v, %q", test.path, kind, drive, test.kind, test.drive)
		}
	}
	kind, drive := resolveRootAlias("", "/Docs")
	if kind != rootAliasDrive || drive != "/Docs" {
		t.Fatalf("resolveRootAlias without alias = %v, %q; want drive path", kind, drive)
	}
}

func TestRootAliasValidation(t *testing.T) {
	for _, alias := range []string{"", testRootAliasFolder, "Drive-2"} {
		if err := validateRootAlias(alias); err != nil {
			t.Fatalf("validateRootAlias(%q) = %v; want nil", alias, err)
		}
	}
	for _, alias := range []string{".", "..", " padded", "a/b", `a\b`, "nul\x00"} {
		if err := validateRootAlias(alias); err == nil {
			t.Fatalf("validateRootAlias(%q) accepted an unsafe alias", alias)
		}
	}
}

func TestRootAliasCapabilityRootListsOnlyAliasFolder(t *testing.T) {
	handler := newRootAliasTestHandler(t, testRootAliasFolder, nil)
	for _, depth := range []string{"0", "1", windowsNoRootDepth} {
		t.Run("depth "+depth, func(t *testing.T) {
			request := trustedRequest("PROPFIND", testCapability+"/", bytes.NewBufferString(`<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`))
			request.Header.Set("Depth", depth)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusMultiStatus {
				t.Fatalf("PROPFIND status = %d; body=%q", recorder.Code, recorder.Body.String())
			}
			hrefs := propfindResponseHrefs(t, recorder.Body.Bytes())
			var want []string
			switch depth {
			case "0":
				want = []string{testCapability + "/"}
			case "1":
				want = []string{testCapability + "/", testCapability + "/Telegram%20Drive/"}
			default:
				want = []string{testCapability + "/Telegram%20Drive/"}
			}
			if !slices.Equal(hrefs, want) {
				t.Fatalf("PROPFIND depth %s hrefs = %q; want %q", depth, hrefs, want)
			}
		})
	}
}

func TestRootAliasFolderPropfindAndGETReadDriveRoot(t *testing.T) {
	handler := newRootAliasTestHandler(t, testRootAliasFolder, nil)

	request := trustedRequest("PROPFIND", aliasTarget("Telegram%20Drive"), bytes.NewBufferString(`<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`))
	request.Header.Set("Depth", windowsNoRootDepth)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMultiStatus {
		t.Fatalf("alias PROPFIND status = %d; body=%q", recorder.Code, recorder.Body.String())
	}
	if got := propfindResponseHrefs(t, recorder.Body.Bytes()); !slices.Equal(got, []string{testCapability + "/Telegram%20Drive/Docs/"}) {
		t.Fatalf("alias PROPFIND hrefs = %q; want the drive's Docs folder under the alias", got)
	}

	request = trustedRequest("GET", aliasTarget("Telegram%20Drive", "Docs", "note.txt"), nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "hello" {
		t.Fatalf("alias GET = %d %q; want 200 hello", recorder.Code, recorder.Body.String())
	}
}

func TestRootAliasHidesDriveRootFromCapabilityRoot(t *testing.T) {
	handler := newRootAliasTestHandler(t, testRootAliasFolder, nil)
	for _, target := range []string{aliasTarget("Docs", "note.txt"), aliasTarget("Docs")} {
		request := trustedRequest("GET", target, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d; want 404 outside the alias", target, recorder.Code)
		}
	}
}

func TestRootAliasDisabledKeepsCapabilityRootAsDrive(t *testing.T) {
	handler := newRootAliasTestHandler(t, "", nil)
	request := trustedRequest("PROPFIND", testCapability+"/", bytes.NewBufferString(`<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`))
	request.Header.Set("Depth", windowsNoRootDepth)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := propfindResponseHrefs(t, recorder.Body.Bytes()); !slices.Equal(got, []string{testCapability + "/Docs/"}) {
		t.Fatalf("no-alias PROPFIND hrefs = %q; want the drive root children", got)
	}
}

func TestRootAliasRefusesCapabilityRootAndFolderMutations(t *testing.T) {
	writer := &recordingWriteCoordinator{}
	handler := newRootAliasTestHandler(t, testRootAliasFolder, writer)
	folderDestination := "http://127.0.0.1:7331" + aliasTarget("Telegram%20Drive", "Renamed")
	rootDestination := "http://127.0.0.1:7331" + aliasTarget("Renamed")
	cases := []struct {
		name   string
		method string
		target string
		header map[string]string
	}{
		{"PUT at capability root", http.MethodPut, aliasTarget("new.txt"), nil},
		{"MKCOL at capability root", "MKCOL", aliasTarget("newdir"), nil},
		{"PUT over alias folder", http.MethodPut, aliasTarget("Telegram%20Drive"), nil},
		{"MKCOL of alias folder", "MKCOL", aliasTarget("Telegram%20Drive"), nil},
		{"DELETE alias folder", http.MethodDelete, aliasTarget("Telegram%20Drive"), nil},
		{"DELETE capability root", http.MethodDelete, testCapability + "/", nil},
		{"MOVE alias folder out of alias", "MOVE", aliasTarget("Telegram%20Drive"), map[string]string{"Destination": folderDestination}},
		{"MOVE drive item to capability root", "MOVE", aliasTarget("Telegram%20Drive", "Docs"), map[string]string{"Destination": rootDestination}},
		{"LOCK capability root", "LOCK", testCapability + "/", nil},
		{"PROPPATCH alias folder", "PROPPATCH", aliasTarget("Telegram%20Drive"), map[string]string{"Content-Type": "application/xml"}},
		{"PUT outside alias under capability", http.MethodPut, aliasTarget("Docs", "x.txt"), nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := trustedRequest(test.method, test.target, nil)
			for key, value := range test.header {
				request.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("%s %s = %d; want 403", test.method, test.target, recorder.Code)
			}
		})
	}
	if writer.putRequest.Path != "" || writer.mkdirRequest.Path != "" || writer.deleteRequest.Path != "" || writer.moveRequest.SourcePath != "" {
		t.Fatalf("refused mutation reached the coordinator: put=%q mkdir=%q delete=%q move=%q",
			writer.putRequest.Path, writer.mkdirRequest.Path, writer.deleteRequest.Path, writer.moveRequest.SourcePath)
	}
}

func TestRootAliasInsideMutationsReachDriveRoot(t *testing.T) {
	writer := &recordingWriteCoordinator{}
	handler := newRootAliasTestHandler(t, testRootAliasFolder, writer)

	request := trustedRequest(http.MethodPut, aliasTarget("Telegram%20Drive", "new.txt"), strings.NewReader("data"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated && recorder.Code != http.StatusNoContent {
		t.Fatalf("PUT inside alias = %d; body=%q", recorder.Code, recorder.Body.String())
	}
	if writer.putRequest.Path != "/new.txt" || string(writer.putBody) != "data" {
		t.Fatalf("PUT reached coordinator at %q with %q; want /new.txt", writer.putRequest.Path, writer.putBody)
	}

	request = trustedRequest("MKCOL", aliasTarget("Telegram%20Drive", "Folder"), nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("MKCOL inside alias = %d", recorder.Code)
	}
	if writer.mkdirRequest.Path != "/Folder" {
		t.Fatalf("MKCOL path = %q; want /Folder", writer.mkdirRequest.Path)
	}

	request = trustedRequest("MOVE", aliasTarget("Telegram%20Drive", "a.txt"), nil)
	request.Header.Set("Destination", "http://127.0.0.1:7331"+aliasTarget("Telegram%20Drive", "b.txt"))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated && recorder.Code != http.StatusNoContent {
		t.Fatalf("MOVE inside alias = %d", recorder.Code)
	}
	if writer.moveRequest.SourcePath != "/a.txt" || writer.moveRequest.DestinationPath != "/b.txt" {
		t.Fatalf("MOVE paths = %q -> %q; want /a.txt -> /b.txt", writer.moveRequest.SourcePath, writer.moveRequest.DestinationPath)
	}

	request = trustedRequest(http.MethodDelete, aliasTarget("Telegram%20Drive", "a.txt"), nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if writer.deleteRequest.Path != "/a.txt" {
		t.Fatalf("DELETE path = %q; want /a.txt (status %d)", writer.deleteRequest.Path, recorder.Code)
	}
}

func TestRootAliasConditionsAreMappedToDrivePaths(t *testing.T) {
	conditions := MutationConditions{DAVIf: []DAVConditionList{
		{ResourcePath: "/" + testRootAliasFolder + "/a.txt"},
	}}
	mapped, ok := driveConditions(testRootAliasFolder, conditions)
	if !ok || mapped.DAVIf[0].ResourcePath != "/a.txt" {
		t.Fatalf("driveConditions() = %+v, %v; want /a.txt", mapped.DAVIf, ok)
	}
	if conditions.DAVIf[0].ResourcePath != "/"+testRootAliasFolder+"/a.txt" {
		t.Fatal("driveConditions modified the caller's conditions")
	}
	outside := MutationConditions{DAVIf: []DAVConditionList{{ResourcePath: "/Docs/a.txt"}}}
	if _, ok := driveConditions(testRootAliasFolder, outside); ok {
		t.Fatal("driveConditions accepted a resource outside the alias")
	}
}

func TestAliasedWriterRefusesPathsOutsideAlias(t *testing.T) {
	inner := &recordingWriteCoordinator{}
	writer := newAliasedWriter(testRootAliasFolder, inner)
	_, err := writer.Put(context.Background(), PutRequest{Path: "/Docs/a.txt"}, strings.NewReader("x"))
	if !errors.Is(err, ErrWriteForbidden) || inner.putRequest.Path != "" {
		t.Fatalf("Put outside alias = %v, path %q; want forbidden without delegation", err, inner.putRequest.Path)
	}
	if newAliasedWriter("", inner) != WriteCoordinator(inner) {
		t.Fatal("aliasedWriter wrapped a writer with no alias")
	}
}

func TestServerStartRootAliasExposesFolderOnly(t *testing.T) {
	isolate_data_dir(t)
	opener := &memoryOpener{data: []byte("alias bytes")}
	server := NewServer()
	status, err := server.Start(context.Background(), StartConfig{
		FS:        testFS(t, opener).fs,
		DriveID:   7,
		RootAlias: testRootAliasFolder,
	})
	if err != nil {
		t.Fatalf("Start with alias: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	resp, err := http.Get(status.URL + "Telegram%20Drive/Docs/note.txt")
	if err != nil {
		t.Fatalf("GET through alias: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "alias bytes" {
		t.Fatalf("GET through alias = %d %q", resp.StatusCode, body)
	}

	resp, err = http.Get(status.URL + "Docs/note.txt")
	if err != nil {
		t.Fatalf("GET without alias segment: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET without alias segment = %d; want 404", resp.StatusCode)
	}
}
