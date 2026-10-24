package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// max_plan_images caps how many photos one plan may analyze.
	max_plan_images = 500
	// max_plan_folders caps the number of folders a plan may create or reuse.
	max_plan_folders = 50
	// organize_batch_size is how many previews go to the model per request.
	organize_batch_size = 8
	// max_folder_walk caps the folders visited while collecting images.
	max_folder_walk = 2000
	// thumb_workers bounds concurrent thumbnail fetches.
	thumb_workers = 3
	// max_folder_name_runes keeps proposed folder names short.
	max_folder_name_runes = 60
	max_instruction_runes = 2000
	// max_batch_attempts retries a batch once when the reply is not valid JSON.
	max_batch_attempts = 2
)

// image_extensions lists file name suffixes treated as photos. Previews come
// from TDrive's own rendition pipeline, so the source format need not decode
// here; prepare_image skips anything that does not decode.
var image_extensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
	".bmp": true, ".heic": true, ".heif": true, ".tif": true, ".tiff": true,
}

// OrganizeProgress is the payload of the ai:organize_progress event.
type OrganizeProgress struct {
	Analyzed int `json:"analyzed"`
	Total    int `json:"total"`
}

// PlanRequest asks for a photo organization plan in one scope.
type PlanRequest struct {
	Scope       Scope  `json:"scope"`
	Instruction string `json:"instruction"`
	ModelID     string `json:"model_id"`
}

// PlannedFile is one proposed move. It is shown to the user for review.
type PlannedFile struct {
	FileID   int64  `json:"file_id"`
	Name     string `json:"name"`
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
	// FromParentID is the folder the file sat in when the plan was made; undo
	// and conflict checks use it. It is never sent to the frontend.
	FromParentID string `json:"-"`
}

// PlannedFolder is one target folder in a plan.
type PlannedFolder struct {
	Name  string        `json:"name"`
	IsNew bool          `json:"is_new"`
	Files []PlannedFile `json:"files"`
	// ExistingFolderID is set when the folder already exists in the scope.
	ExistingFolderID string `json:"-"`
}

// Plan is a reviewable organization proposal kept in memory until applied.
type Plan struct {
	PlanID  string          `json:"plan_id"`
	Scope   Scope           `json:"scope"`
	Total   int             `json:"total"`
	Folders []PlannedFolder `json:"folders"`
	// Skipped counts photos that had no usable preview and were left alone.
	Skipped int      `json:"skipped"`
	Notes   []string `json:"notes"`

	applied    bool
	created_ms int64
}

// image_item is one photo found while walking the scope.
type image_item struct {
	file     FileNode
	rel_path string
}

// batch_entry is one photo prepared for a model request.
type batch_entry struct {
	file_id int64
	name    string
	jpeg    []byte
}

// organize_reply is the strict JSON shape the model must return.
type organize_reply struct {
	Folders []organize_folder `json:"folders"`
}

type organize_folder struct {
	Name    string    `json:"name"`
	FileIDs []flex_id `json:"file_ids"`
}

// flex_id accepts a JSON integer or a quoted integer, since models vary.
type flex_id int64

func (f *flex_id) UnmarshalJSON(data []byte) error {
	text := strings.Trim(strings.TrimSpace(string(data)), `"`)
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("file id %q is not an integer", text)
	}
	*f = flex_id(value)
	return nil
}

// batch_folder is one validated folder from a single model reply.
type batch_folder struct {
	name     string
	file_ids []int64
}

// folder_draft accumulates the files assigned to one folder name across batches.
type folder_draft struct {
	key      string
	name     string
	file_ids []int64
}

// PlanOrganize analyzes the photos in the scope with a vision model and returns
// a plan. Nothing moves until ApplyPlan. Progress is sent as
// ai:organize_progress events while the call is in flight.
func (s *Service) PlanOrganize(ctx context.Context, req PlanRequest) (Plan, error) {
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		return Plan{}, errors.New("ai: describe how the photos should be organized")
	}
	if utf8.RuneCountInString(instruction) > max_instruction_runes {
		return Plan{}, fmt.Errorf("ai: instruction must be at most %d characters", max_instruction_runes)
	}
	if req.Scope.ChannelID == 0 {
		return Plan{}, errors.New("ai: no drive selected")
	}
	drive, err := s.drive()
	if err != nil {
		return Plan{}, err
	}
	if err := drive.CanOrganize(req.Scope.ChannelID); err != nil {
		return Plan{}, err
	}
	manager, err := s.models()
	if err != nil {
		return Plan{}, err
	}
	runtime, err := manager.Resolve(req.ModelID)
	if err != nil {
		return Plan{}, err
	}
	if !runtime.Profile.VisionEnabled {
		return Plan{}, fmt.Errorf("ai: model %q is not marked as vision-enabled; choose a model that can see images", runtime.Profile.DisplayName)
	}
	completer, err := NewCompleter(runtime, s.opts.HTTPClient)
	if err != nil {
		return Plan{}, err
	}

	items, root_children, err := collect_images(ctx, drive, req.Scope, max_folder_walk)
	if err != nil {
		return Plan{}, err
	}
	if len(items) == 0 {
		return Plan{}, errors.New("ai: no photos found in this folder")
	}
	s.emit(EventOrganizeProgress, OrganizeProgress{Analyzed: 0, Total: len(items)})

	thumbs, err := fetch_thumbnails(ctx, drive, req.Scope.ChannelID, items)
	if err != nil {
		return Plan{}, err
	}
	drafts, skipped, notes, err := s.analyze_batches(ctx, completer, instruction, items, thumbs)
	if err != nil {
		return Plan{}, err
	}
	folders, plan_notes := finalize_folders(drafts, items, root_children)
	notes = append(notes, plan_notes...)

	total := 0
	for _, folder := range folders {
		total += len(folder.Files)
	}
	plan := Plan{
		PlanID:     uuid.NewString(),
		Scope:      req.Scope,
		Total:      total,
		Folders:    folders,
		Skipped:    skipped,
		Notes:      notes,
		created_ms: s.now().UnixMilli(),
	}
	if plan.Folders == nil {
		plan.Folders = []PlannedFolder{}
	}
	if plan.Notes == nil {
		plan.Notes = []string{}
	}
	s.store_plan(&plan)
	return plan, nil
}

// collect_images walks the scope breadth first and returns every photo, plus the
// direct child folders of the scope root (used to match existing folder names).
func collect_images(ctx context.Context, drive Drive, scope Scope, walk_limit int) ([]image_item, []FolderNode, error) {
	type queued struct {
		folder_id string
		rel       string
	}
	var root_children []FolderNode
	var items []image_item
	queue := []queued{{folder_id: scope.FolderID}}
	visited := map[string]bool{scope.FolderID: true}
	visits := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		current := queue[0]
		queue = queue[1:]
		visits++
		if visits > walk_limit {
			return nil, nil, errors.New("ai: too many folders to scan; choose a smaller folder")
		}
		folders, files, err := drive.ListFolder(ctx, scope.ChannelID, current.folder_id)
		if err != nil {
			return nil, nil, err
		}
		if visits == 1 {
			root_children = folders
		}
		for _, file := range files {
			if is_image_name(file.Name) {
				items = append(items, image_item{file: file, rel_path: path.Join(current.rel, file.Name)})
			}
		}
		if len(items) > max_plan_images {
			return nil, nil, fmt.Errorf("ai: found more than %d photos; choose a smaller folder", max_plan_images)
		}
		for _, folder := range folders {
			if visited[folder.ID] {
				continue
			}
			visited[folder.ID] = true
			queue = append(queue, queued{folder_id: folder.ID, rel: path.Join(current.rel, folder.Name)})
		}
	}
	if root_children == nil {
		root_children = []FolderNode{}
	}
	return items, root_children, nil
}

func is_image_name(name string) bool {
	return image_extensions[strings.ToLower(path.Ext(name))]
}

// fetch_thumbnails returns one prepared JPEG per item, or nil where the preview
// could not be produced. A missing preview is not an error for the whole plan.
func fetch_thumbnails(ctx context.Context, drive Drive, channel_id int64, items []image_item) ([][]byte, error) {
	out := make([][]byte, len(items))
	sem := make(chan struct{}, thumb_workers)
	done := make(chan struct{})
	pending := 0
	for i := range items {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		pending++
		go func(i int) {
			defer func() {
				<-sem
				done <- struct{}{}
			}()
			raw, err := drive.Thumbnail(ctx, channel_id, items[i].file.ID)
			if err != nil {
				return
			}
			jpeg, err := prepare_image(raw)
			if err != nil {
				return
			}
			out[i] = jpeg
		}(i)
	}
	for ; pending > 0; pending-- {
		<-done
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// analyze_batches sends previews to the model in batches and merges the
// validated folder assignments. A batch that fails twice aborts the plan.
func (s *Service) analyze_batches(
	ctx context.Context,
	completer Completer,
	instruction string,
	items []image_item,
	thumbs [][]byte,
) ([]folder_draft, int, []string, error) {
	var drafts []folder_draft
	draft_index := map[string]int{}
	taken := map[int64]bool{}
	var notes []string
	skipped := 0
	analyzed := 0
	total := len(items)
	for start := 0; start < total; start += organize_batch_size {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}
		end := min(start+organize_batch_size, total)
		var entries []batch_entry
		allowed := map[int64]bool{}
		for i := start; i < end; i++ {
			if thumbs[i] == nil {
				skipped++
				continue
			}
			entries = append(entries, batch_entry{file_id: items[i].file.ID, name: items[i].file.Name, jpeg: thumbs[i]})
			allowed[items[i].file.ID] = true
		}
		if len(entries) > 0 {
			known := known_folder_names(drafts)
			reply, err := request_organize_batch(ctx, completer, instruction, known, entries)
			if err != nil {
				return nil, 0, nil, err
			}
			folders, dropped := validate_batch(reply, allowed, taken)
			if dropped > 0 {
				notes = append(notes, fmt.Sprintf("Ignored %d file assignment(s) the model returned that were unknown or repeated.", dropped))
			}
			for _, folder := range folders {
				key := strings.ToLower(folder.name)
				index, ok := draft_index[key]
				if !ok {
					index = len(drafts)
					draft_index[key] = index
					drafts = append(drafts, folder_draft{key: key, name: folder.name})
				}
				drafts[index].file_ids = append(drafts[index].file_ids, folder.file_ids...)
				for _, id := range folder.file_ids {
					taken[id] = true
				}
			}
		}
		analyzed = end
		s.emit(EventOrganizeProgress, OrganizeProgress{Analyzed: analyzed, Total: total})
	}
	return drafts, skipped, notes, nil
}

// known_folder_names lists folder names already proposed, so later batches reuse
// them instead of inventing near-duplicates.
func known_folder_names(drafts []folder_draft) []string {
	names := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		names = append(names, draft.name)
	}
	return names
}

// request_organize_batch asks the model for folder assignments for one batch,
// retrying once when the reply is not parseable JSON.
func request_organize_batch(ctx context.Context, completer Completer, instruction string, known []string, entries []batch_entry) (organize_reply, error) {
	var last_err error
	for attempt := 0; attempt < max_batch_attempts; attempt++ {
		reply, err := completer.Complete(ctx, build_organize_request(instruction, known, entries))
		if err != nil {
			return organize_reply{}, err
		}
		parsed, err := parse_organize_reply(reply.Text)
		if err == nil {
			return parsed, nil
		}
		last_err = err
	}
	return organize_reply{}, fmt.Errorf("ai: the model did not return a valid organize plan: %w", last_err)
}

// build_organize_request builds the provider request for one batch.
func build_organize_request(instruction string, known []string, entries []batch_entry) Request {
	var system strings.Builder
	system.WriteString("You sort photos into folders for the user. Reply with one JSON object and nothing else, shaped exactly as:\n")
	system.WriteString(`{"folders":[{"name":"Folder name","file_ids":[123,456]}]}` + "\n")
	system.WriteString("Rules:\n")
	system.WriteString("- Use only file_ids listed in the user message. Each file_id may appear at most once across all folders.\n")
	system.WriteString("- Omit photos that do not fit the instruction.\n")
	system.WriteString("- Folder names are short (at most 60 characters) and must not contain / \\ : * ? \" < > |.\n")
	system.WriteString(fmt.Sprintf("- Use at most %d folders in total.\n", max_plan_folders))
	if len(known) > 0 {
		system.WriteString("- Reuse these folder names when they fit: " + strings.Join(known, "; ") + ".\n")
	}
	var text strings.Builder
	text.WriteString("Instruction: " + instruction + "\n\nPhotos in this batch (file_id and name). Images follow in the same order:\n")
	turn := Turn{Role: "user"}
	for _, entry := range entries {
		text.WriteString(fmt.Sprintf("- file_id %d: %s\n", entry.file_id, entry.name))
		turn.Images = append(turn.Images, Image{MIMEType: "image/jpeg", Data: entry.jpeg})
	}
	turn.Text = text.String()
	return Request{System: system.String(), Turns: []Turn{turn}, JSONOutput: true, MaxTokens: 1024}
}

// parse_organize_reply extracts the JSON object from a model reply. Code fences
// and surrounding prose are tolerated; the object itself must be well formed.
func parse_organize_reply(text string) (organize_reply, error) {
	candidate := strings.TrimSpace(text)
	if strings.HasPrefix(candidate, "```") {
		candidate = strings.TrimPrefix(candidate, "```json")
		candidate = strings.TrimPrefix(candidate, "```")
		candidate = strings.TrimSuffix(strings.TrimSpace(candidate), "```")
	}
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start < 0 || end < start {
		return organize_reply{}, errors.New("reply contains no JSON object")
	}
	var reply organize_reply
	if err := json.Unmarshal([]byte(candidate[start:end+1]), &reply); err != nil {
		return organize_reply{}, fmt.Errorf("reply is not valid JSON: %w", err)
	}
	if reply.Folders == nil {
		return organize_reply{}, errors.New("reply has no folders array")
	}
	return reply, nil
}

// validate_batch keeps only assignments that name an allowed, not yet taken file
// and a usable folder name. It returns the valid folders and how many file
// assignments were dropped.
func validate_batch(reply organize_reply, allowed map[int64]bool, taken map[int64]bool) ([]batch_folder, int) {
	var folders []batch_folder
	dropped := 0
	seen := map[int64]bool{}
	for _, folder := range reply.Folders {
		name, ok := sanitize_folder_name(folder.Name)
		if !ok {
			dropped += len(folder.FileIDs)
			continue
		}
		var ids []int64
		for _, raw := range folder.FileIDs {
			id := int64(raw)
			if !allowed[id] || taken[id] || seen[id] {
				dropped++
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			folders = append(folders, batch_folder{name: name, file_ids: ids})
		}
	}
	return folders, dropped
}

// sanitize_folder_name makes a model-proposed name safe as a single folder name.
// It reports false when nothing usable is left.
func sanitize_folder_name(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteRune(' ')
		case strings.ContainsRune(`\/:*?"<>|`, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(name) > max_folder_name_runes {
		name = string([]rune(name)[:max_folder_name_runes])
	}
	name = strings.Trim(name, ". ")
	if name == "" {
		return "", false
	}
	if is_reserved_name(name) {
		name += "_"
	}
	return name, true
}

// is_reserved_name reports Windows device names that cannot be folder names.
func is_reserved_name(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		return upper[3] >= '1' && upper[3] <= '9'
	}
	return false
}

// finalize_folders turns merged drafts into plan folders. It matches names to
// folders that already exist in the scope, drops files already in their target,
// and enforces the folder cap.
func finalize_folders(drafts []folder_draft, items []image_item, root_children []FolderNode) ([]PlannedFolder, []string) {
	var notes []string
	existing := map[string]FolderNode{}
	for _, folder := range root_children {
		existing[strings.ToLower(folder.Name)] = folder
	}
	if len(drafts) > max_plan_folders {
		left := 0
		for _, draft := range drafts[max_plan_folders:] {
			left += len(draft.file_ids)
		}
		notes = append(notes, fmt.Sprintf("Kept the first %d folders; %d photo(s) stay where they are.", max_plan_folders, left))
		drafts = drafts[:max_plan_folders]
	}
	by_id := make(map[int64]image_item, len(items))
	for _, item := range items {
		by_id[item.file.ID] = item
	}
	folders := make([]PlannedFolder, 0, len(drafts))
	for _, draft := range drafts {
		folder := PlannedFolder{Name: draft.name, IsNew: true}
		match, has_match := existing[draft.key]
		if has_match {
			folder.Name = match.Name
			folder.IsNew = false
			folder.ExistingFolderID = match.ID
		}
		for _, id := range draft.file_ids {
			item := by_id[id]
			if has_match && item.file.ParentID == match.ID {
				continue
			}
			folder.Files = append(folder.Files, PlannedFile{
				FileID:       item.file.ID,
				Name:         item.file.Name,
				FromPath:     item.rel_path,
				ToPath:       path.Join(folder.Name, item.file.Name),
				FromParentID: item.file.ParentID,
			})
		}
		if len(folder.Files) > 0 {
			folders = append(folders, folder)
		}
	}
	return folders, notes
}

// store_plan keeps a plan for ApplyPlan and drops expired plans.
func (s *Service) store_plan(plan *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune_locked(s.now().UnixMilli())
	s.plans[plan.PlanID] = plan
}
