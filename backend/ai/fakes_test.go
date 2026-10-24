package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// fake_secrets is an in-memory SecretStore.
type fake_secrets struct {
	mu   sync.Mutex
	keys map[string]string
}

func new_fake_secrets() *fake_secrets {
	return &fake_secrets{keys: map[string]string{}}
}

func (f *fake_secrets) Set(id, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[id] = key
	return nil
}

func (f *fake_secrets) Get(id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key, ok := f.keys[id]
	if !ok {
		return "", ErrNoAPIKey
	}
	return key, nil
}

func (f *fake_secrets) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, id)
	return nil
}

// fake_drive is an in-memory Drive. Files and folders keep their current parent
// so MoveFile is observable through ListFolder and ParentOf.
type fake_drive struct {
	mu          sync.Mutex
	read_only   bool
	folders     []FolderNode
	files       []FileNode
	thumbs      map[int64][]byte
	fail_move   map[int64]bool
	next_folder int
	moves       []fake_move
	created     []string
}

type fake_move struct {
	file_id int64
	parent  string
}

func (d *fake_drive) CanOrganize(channel_id int64) error {
	if d.read_only {
		return errors.New("channel source is read-only")
	}
	return nil
}

func (d *fake_drive) ListFolder(ctx context.Context, channel_id int64, folder_id string) ([]FolderNode, []FileNode, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var folders []FolderNode
	for _, folder := range d.folders {
		if folder.ParentID == folder_id {
			folders = append(folders, folder)
		}
	}
	var files []FileNode
	for _, file := range d.files {
		if file.ParentID == folder_id {
			files = append(files, file)
		}
	}
	return folders, files, nil
}

func (d *fake_drive) Thumbnail(ctx context.Context, channel_id int64, file_id int64) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	raw, ok := d.thumbs[file_id]
	if !ok {
		return nil, errors.New("no preview")
	}
	return raw, nil
}

func (d *fake_drive) CreateFolder(ctx context.Context, channel_id int64, name string, parent_id string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next_folder++
	id := fmt.Sprintf("new-%d", d.next_folder)
	d.folders = append(d.folders, FolderNode{ID: id, Name: name, ParentID: parent_id})
	d.created = append(d.created, name)
	return id, nil
}

func (d *fake_drive) ParentOf(ctx context.Context, channel_id int64, file_id int64) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, file := range d.files {
		if file.ID == file_id {
			return file.ParentID, nil
		}
	}
	return "", errors.New("file not found")
}

func (d *fake_drive) MoveFile(ctx context.Context, channel_id int64, file_id int64, parent_id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail_move[file_id] {
		return errors.New("telegram refused the move")
	}
	for i := range d.files {
		if d.files[i].ID == file_id {
			d.files[i].ParentID = parent_id
			d.moves = append(d.moves, fake_move{file_id: file_id, parent: parent_id})
			return nil
		}
	}
	return errors.New("file not found")
}

// file_parent is a test helper that reads a file's current parent.
func (d *fake_drive) file_parent(id int64) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, file := range d.files {
		if file.ID == id {
			return file.ParentID
		}
	}
	return "missing"
}

// png_bytes encodes a solid image of the given size as PNG.
func png_bytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// fake_vision_server answers OpenAI-style chat requests. For every "file_id N:
// name" line in the user text it assigns the file to a folder chosen by a rule
// on the name, and it records how many images each request carried.
type fake_vision_server struct {
	server     *httptest.Server
	mu         sync.Mutex
	requests   int
	image_seen []int
	bad_json   bool
	status     int
	rule       func(name string) string
}

var file_line = regexp.MustCompile(`file_id (\d+): (\S+)`)

func new_fake_vision_server(t *testing.T, rule func(name string) string) *fake_vision_server {
	t.Helper()
	fake := &fake_vision_server{rule: rule}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.requests++
		fake.mu.Unlock()
		if fake.status != 0 {
			w.WriteHeader(fake.status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
			return
		}
		var req struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		last := req.Messages[len(req.Messages)-1].Content
		var text strings.Builder
		images := 0
		if parts, ok := last.([]any); ok {
			for _, part := range parts {
				m := part.(map[string]any)
				switch m["type"] {
				case "text":
					text.WriteString(m["text"].(string))
				case "image_url":
					images++
				}
			}
		} else {
			text.WriteString(fmt.Sprint(last))
		}
		fake.mu.Lock()
		fake.image_seen = append(fake.image_seen, images)
		fake.mu.Unlock()
		if fake.bad_json {
			write_chat_reply(w, "sorry, I cannot do that")
			return
		}
		groups := map[string][]string{}
		var order []string
		for _, match := range file_line.FindAllStringSubmatch(text.String(), -1) {
			folder := fake.rule(match[2])
			if _, ok := groups[folder]; !ok {
				order = append(order, folder)
			}
			groups[folder] = append(groups[folder], match[1])
		}
		var folders []string
		for _, name := range order {
			folders = append(folders, fmt.Sprintf(`{"name":%q,"file_ids":[%s]}`, name, strings.Join(groups[name], ",")))
		}
		write_chat_reply(w, `{"folders":[`+strings.Join(folders, ",")+`]}`)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func write_chat_reply(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	reply := map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"content": content}}},
	}
	_ = json.NewEncoder(w).Encode(reply)
}

func (f *fake_vision_server) request_count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}
