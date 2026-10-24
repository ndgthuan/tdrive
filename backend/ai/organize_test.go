package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"strings"
	"sync"
	"testing"
)

// photo_drive builds the fixture used by the planner and apply tests:
//
//	root: beach_1.jpg, beach_2.jpg (already in Beach), city_1.jpg, city_2.jpg,
//	      food_1.jpg (no preview), readme.txt (not a photo), folder Beach, folder Trips
//	Trips: city_3.jpg, beach_9.jpg
func photo_drive(t *testing.T) *fake_drive {
	t.Helper()
	preview := png_bytes(t, 64, 48)
	return &fake_drive{
		folders: []FolderNode{
			{ID: "beach", Name: "Beach", ParentID: ""},
			{ID: "trips", Name: "Trips", ParentID: ""},
		},
		files: []FileNode{
			{ID: 1, Name: "beach_1.jpg", ParentID: ""},
			{ID: 2, Name: "beach_2.jpg", ParentID: "beach"},
			{ID: 3, Name: "city_1.jpg", ParentID: ""},
			{ID: 4, Name: "city_2.jpg", ParentID: ""},
			{ID: 5, Name: "food_1.jpg", ParentID: ""},
			{ID: 6, Name: "nopreview.jpg", ParentID: ""},
			{ID: 7, Name: "city_3.jpg", ParentID: "trips"},
			{ID: 8, Name: "beach_9.jpg", ParentID: "trips"},
			{ID: 9, Name: "readme.txt", ParentID: ""},
		},
		thumbs: map[int64][]byte{
			1: preview, 2: preview, 3: preview, 4: preview, 5: preview, 7: preview, 8: preview,
		},
		fail_move: map[int64]bool{},
	}
}

// photo_rule sorts by the first word of the file name.
func photo_rule(name string) string {
	switch {
	case strings.HasPrefix(name, "beach"):
		return "beach"
	case strings.HasPrefix(name, "city"):
		return "City"
	default:
		return "Food"
	}
}

type event_log struct {
	mu     sync.Mutex
	events []string
	values []any
}

func (e *event_log) emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, name)
	e.values = append(e.values, payload)
}

func (e *event_log) names(name string) []any {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []any
	for i, n := range e.events {
		if n == name {
			out = append(out, e.values[i])
		}
	}
	return out
}

func new_organize_service(t *testing.T, drive *fake_drive, vision_url string, vision bool) (*Service, *event_log) {
	t.Helper()
	isolate_data_dir(t)
	log := &event_log{}
	svc := NewService(Options{
		Secrets: new_fake_secrets(),
		Drive:   func() (Drive, error) { return drive, nil },
		Emit:    log.emit,
	})
	if _, err := svc.AddModel(ModelInput{
		DisplayName:   "Vision",
		Provider:      ProviderCustom,
		BaseURL:       vision_url,
		Model:         "vision-model",
		VisionEnabled: vision,
	}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	return svc, log
}

func plan_request(model_id string) PlanRequest {
	return PlanRequest{Scope: Scope{ChannelID: 42}, Instruction: "group by place", ModelID: model_id}
}

func TestPlanOrganizeBuildsReviewablePlan(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	svc, log := new_organize_service(t, drive, fake.server.URL+"/v1", true)

	plan, err := svc.PlanOrganize(context.Background(), plan_request(""))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.PlanID == "" || plan.Skipped != 1 {
		t.Fatalf("plan id/skipped wrong: %+v", plan)
	}
	if fake.request_count() != 1 {
		t.Fatalf("8 photos fit in one batch, got %d requests", fake.request_count())
	}
	if fake.image_seen[0] != 7 {
		t.Fatalf("model should receive 7 images (one has no preview), got %d", fake.image_seen[0])
	}

	by_name := map[string]PlannedFolder{}
	for _, folder := range plan.Folders {
		by_name[folder.Name] = folder
	}
	beach, ok := by_name["Beach"]
	if !ok || beach.IsNew || beach.ExistingFolderID != "beach" {
		t.Fatalf("Beach should reuse the existing folder, got %+v", beach)
	}
	if got := file_ids(beach.Files); got != "1,8" {
		t.Fatalf("Beach files = %s, want 1,8 (beach_2 is already there)", got)
	}
	city, ok := by_name["City"]
	if !ok || !city.IsNew {
		t.Fatalf("City should be a new folder, got %+v", city)
	}
	if got := file_ids(city.Files); got != "3,4,7" {
		t.Fatalf("City files = %s, want 3,4,7", got)
	}
	if city.Files[0].ToPath != "City/city_1.jpg" || city.Files[2].FromPath != "Trips/city_3.jpg" {
		t.Fatalf("paths wrong: %+v", city.Files)
	}
	if food := by_name["Food"]; file_ids(food.Files) != "5" || !food.IsNew {
		t.Fatalf("Food folder wrong: %+v", food)
	}
	if plan.Total != 6 {
		t.Fatalf("total = %d, want 6", plan.Total)
	}

	progress := log.names("ai:organize_progress")
	if len(progress) < 2 {
		t.Fatalf("expected progress events, got %d", len(progress))
	}
	last := progress[len(progress)-1].(OrganizeProgress)
	if last.Analyzed != 8 || last.Total != 8 {
		t.Fatalf("final progress = %+v", last)
	}
}

func TestPlanOrganizeRejectsNonVisionModel(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", false)
	if _, err := svc.PlanOrganize(context.Background(), plan_request("")); err == nil || !strings.Contains(err.Error(), "vision") {
		t.Fatalf("non-vision model must be refused before any request, got %v", err)
	}
	if fake.request_count() != 0 {
		t.Fatalf("no provider request should be made")
	}
}

func TestPlanOrganizeRefusesReadOnlySource(t *testing.T) {
	drive := photo_drive(t)
	drive.read_only = true
	fake := new_fake_vision_server(t, photo_rule)
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	if _, err := svc.PlanOrganize(context.Background(), plan_request("")); err == nil {
		t.Fatalf("read-only source must not be organized")
	}
	if fake.request_count() != 0 {
		t.Fatalf("no model request should be made for a read-only source")
	}
}

func TestPlanOrganizeRetriesThenFailsOnBadJSON(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	fake.bad_json = true
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	_, err := svc.PlanOrganize(context.Background(), plan_request(""))
	if err == nil || !strings.Contains(err.Error(), "valid organize plan") {
		t.Fatalf("bad JSON should fail with a clear error, got %v", err)
	}
	if fake.request_count() != max_batch_attempts {
		t.Fatalf("expected %d attempts, got %d", max_batch_attempts, fake.request_count())
	}
}

func TestPlanOrganizeSurfacesProviderAuthFailure(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	fake.status = 401
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	if _, err := svc.PlanOrganize(context.Background(), plan_request("")); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("401 should map to invalid API key, got %v", err)
	}
}

func TestPlanOrganizeRejectsEmptyFolderAndEmptyInstruction(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	if _, err := svc.PlanOrganize(context.Background(), PlanRequest{Scope: Scope{ChannelID: 42}, Instruction: "  "}); err == nil {
		t.Fatalf("blank instruction must be refused")
	}
	empty := &fake_drive{fail_move: map[int64]bool{}}
	svc2, _ := new_organize_service(t, empty, fake.server.URL+"/v1", true)
	if _, err := svc2.PlanOrganize(context.Background(), plan_request("")); err == nil || !strings.Contains(err.Error(), "no photos") {
		t.Fatalf("empty folder should report no photos, got %v", err)
	}
}

func TestValidateBatchDropsUnknownRepeatedAndTaken(t *testing.T) {
	reply := organize_reply{Folders: []organize_folder{
		{Name: "Trips", FileIDs: []flex_id{1, 2, 99}},
		{Name: "Trips again", FileIDs: []flex_id{2, 3}},
		{Name: "", FileIDs: []flex_id{4}},
		{Name: "CON", FileIDs: []flex_id{5}},
	}}
	allowed := map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	taken := map[int64]bool{3: true}
	folders, dropped := validate_batch(reply, allowed, taken)
	if dropped != 4 {
		t.Fatalf("dropped = %d, want 4 (99 unknown, 2 repeated, 3 taken, 4 unnamed)", dropped)
	}
	if len(folders) != 2 {
		t.Fatalf("want 2 usable folders, got %d", len(folders))
	}
	if folders[0].name != "Trips" || len(folders[0].file_ids) != 2 {
		t.Fatalf("first folder wrong: %+v", folders[0])
	}
	if folders[1].name != "CON_" {
		t.Fatalf("reserved name should be suffixed, got %q", folders[1].name)
	}
}

func TestParseOrganizeReplyToleratesFencesAndStringIDs(t *testing.T) {
	text := "Here you go:\n```json\n{\"folders\":[{\"name\":\"Cats\",\"file_ids\":[\"7\", 8]}]}\n```"
	reply, err := parse_organize_reply(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(reply.Folders) != 1 || int64(reply.Folders[0].FileIDs[0]) != 7 || int64(reply.Folders[0].FileIDs[1]) != 8 {
		t.Fatalf("parsed reply wrong: %+v", reply)
	}
	for _, bad := range []string{"no json here", `{"items":[]}`, `{"folders":"x"}`, `{"folders":[{"name":"a","file_ids":["x"]}]}`} {
		if _, err := parse_organize_reply(bad); err == nil {
			t.Errorf("expected parse error for %q", bad)
		}
	}
}

func TestSanitizeFolderName(t *testing.T) {
	cases := map[string]string{
		"  Beach / Sunset  ":  "Beach Sunset",
		"a:b*c?d":             "a b c d",
		"...trailing dots...": "trailing dots",
		"NUL":                 "NUL_",
		"COM3":                "COM3_",
		"COM":                 "COM",
	}
	for input, want := range cases {
		got, ok := sanitize_folder_name(input)
		if !ok || got != want {
			t.Errorf("sanitize(%q) = %q,%v want %q", input, got, ok, want)
		}
	}
	if _, ok := sanitize_folder_name(" ... / "); ok {
		t.Errorf("empty-after-cleaning name must be rejected")
	}
	long := strings.Repeat("x", 200)
	if got, _ := sanitize_folder_name(long); len([]rune(got)) != max_folder_name_runes {
		t.Errorf("long name should be truncated to %d runes", max_folder_name_runes)
	}
}

func TestFinalizeFoldersCapsCountAndMatchesExisting(t *testing.T) {
	var drafts []folder_draft
	items := []image_item{}
	for i := 0; i < max_plan_folders+3; i++ {
		id := int64(1000 + i)
		items = append(items, image_item{file: FileNode{ID: id, Name: fmt.Sprintf("p%d.jpg", i)}})
		drafts = append(drafts, folder_draft{key: fmt.Sprintf("f%d", i), name: fmt.Sprintf("F%d", i), file_ids: []int64{id}})
	}
	root := []FolderNode{{ID: "existing", Name: "F0"}}
	folders, notes := finalize_folders(drafts, items, root)
	if len(folders) != max_plan_folders {
		t.Fatalf("folders = %d, want cap %d", len(folders), max_plan_folders)
	}
	if len(notes) != 1 {
		t.Fatalf("expected one cap note, got %v", notes)
	}
	if folders[0].IsNew || folders[0].ExistingFolderID != "existing" {
		t.Fatalf("F0 should match the existing folder: %+v", folders[0])
	}
}

func TestPrepareImageDownscalesToJPEG(t *testing.T) {
	raw := png_bytes(t, 2000, 1000)
	out, err := prepare_image(raw)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not JPEG: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != thumb_max_side || bounds.Dy() != thumb_max_side/2 {
		t.Fatalf("size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), thumb_max_side, thumb_max_side/2)
	}
	if _, err := prepare_image([]byte("not an image")); err == nil {
		t.Fatalf("garbage bytes must be rejected")
	}

}

func file_ids(files []PlannedFile) string {
	var parts []string
	for _, file := range files {
		parts = append(parts, fmt.Sprint(file.FileID))
	}
	return strings.Join(parts, ",")
}
