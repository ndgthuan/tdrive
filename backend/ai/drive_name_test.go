package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestChatSystemPromptNamesTheActiveDrive(t *testing.T) {
	shared := chat_system_prompt(Scope{ChannelID: 42}, "temp")
	if !strings.Contains(shared, `the root of the drive named "temp"`) {
		t.Fatalf("shared drive root should be named, got:\n%s", shared)
	}
	folder := chat_system_prompt(Scope{ChannelID: 42, FolderID: "f1"}, "temp")
	if !strings.Contains(folder, `a folder inside the drive named "temp"`) {
		t.Fatalf("folder should sit inside the named drive, got:\n%s", folder)
	}
	personal := chat_system_prompt(Scope{ChannelID: 7}, "My Drive")
	if !strings.Contains(personal, `the drive named "My Drive"`) {
		t.Fatalf("personal drive should keep its own name, got:\n%s", personal)
	}
}

func TestChatSystemPromptFallsBackToDriveIDWhenNameUnknown(t *testing.T) {
	prompt := chat_system_prompt(Scope{ChannelID: 42}, "")
	if !strings.Contains(prompt, "the root of drive 42") {
		t.Fatalf("unknown drive should fall back to its id, got:\n%s", prompt)
	}
}

func TestSendMessageUsesTheScopeDriveName(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	isolate_data_dir(t)
	drive := &fake_drive{drive_name: "temp"}
	svc := NewService(Options{
		Secrets: new_fake_secrets(),
		Drive:   func() (Drive, error) { return drive, nil },
	})
	if _, err := svc.AddModel(ModelInput{DisplayName: "Chat", Provider: ProviderCustom, BaseURL: server.URL + "/v1", Model: "chat-model"}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	session, err := svc.CreateSession("", Scope{ChannelID: 9})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SendMessage(context.Background(), SendMessageRequest{SessionID: session.ID, Text: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if body := string(captured.body); !strings.Contains(body, "the drive named") || !strings.Contains(body, "temp") {
		t.Fatalf("prompt should name the shared drive, body: %s", body)
	}
}

func TestSendMessageFallsBackWhenDriveUnavailable(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	isolate_data_dir(t)
	svc := NewService(Options{
		Secrets: new_fake_secrets(),
		Drive:   func() (Drive, error) { return nil, errors.New("not ready") },
	})
	if _, err := svc.AddModel(ModelInput{DisplayName: "Chat", Provider: ProviderCustom, BaseURL: server.URL + "/v1", Model: "chat-model"}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	session, err := svc.CreateSession("", Scope{ChannelID: 9})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SendMessage(context.Background(), SendMessageRequest{SessionID: session.ID, Text: "hi"}); err != nil {
		t.Fatalf("send should still work without a drive name: %v", err)
	}
	if !strings.Contains(string(captured.body), "drive 9") {
		t.Fatalf("prompt should fall back to the drive id")
	}
}
