package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func new_chat_service(t *testing.T, base_url string, log *event_log) *Service {
	t.Helper()
	isolate_data_dir(t)
	svc := NewService(Options{
		Secrets: new_fake_secrets(),
		Emit:    log.emit,
	})
	if _, err := svc.AddModel(ModelInput{
		DisplayName: "Chat",
		Provider:    ProviderCustom,
		BaseURL:     base_url,
		Model:       "chat-model",
	}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	return svc
}

func TestSessionCreateListGetDelete(t *testing.T) {
	isolate_data_dir(t)
	svc := NewService(Options{Secrets: new_fake_secrets()})

	created, err := svc.CreateSession("", Scope{ChannelID: 7, FolderID: "photos"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != "New chat" || created.Scope.FolderID != "photos" {
		t.Fatalf("created session wrong: %+v", created)
	}
	list, err := svc.ListSessions()
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	got, err := svc.GetSession(created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := svc.GetSession("../secrets"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("path-like ids must be rejected as not found, got %v", err)
	}
	if err := svc.DeleteSession(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetSession(created.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("deleted session should be gone, got %v", err)
	}
}

func TestSendMessagePersistsTurnsAndEmitsEvents(t *testing.T) {
	server, captured := capture_server(t, http.StatusOK, `{"choices":[{"message":{"content":"Hello from the model"}}]}`)
	log := &event_log{}
	svc := new_chat_service(t, server.URL+"/v1", log)

	session, err := svc.CreateSession("", Scope{ChannelID: 9})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	reply, err := svc.SendMessage(context.Background(), SendMessageRequest{
		SessionID: session.ID,
		Text:      "What can you do here?",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if reply.Role != "assistant" || reply.Text != "Hello from the model" || reply.ModelID == "" {
		t.Fatalf("reply = %+v", reply)
	}

	body := string(captured.body)
	if !strings.Contains(body, "TDrive") || !strings.Contains(body, "Organize photos") {
		t.Fatalf("system prompt should describe TDrive and the organize capability")
	}
	if !strings.Contains(body, "What can you do here?") {
		t.Fatalf("user text should reach the provider")
	}

	stored, err := svc.GetSession(session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.Messages) != 2 || stored.Messages[0].Role != "user" || stored.Messages[1].Text != "Hello from the model" {
		t.Fatalf("stored messages = %+v", stored.Messages)
	}
	if stored.Title != "What can you do here?" {
		t.Fatalf("title should come from the first message, got %q", stored.Title)
	}

	deltas := log.names(EventMessageDelta)
	done := log.names(EventMessageDone)
	if len(deltas) != 1 || len(done) != 1 {
		t.Fatalf("expected one delta and one done event, got %d and %d", len(deltas), len(done))
	}
	if d := deltas[0].(MessageDelta); d.Delta != "Hello from the model" || d.SessionID != session.ID {
		t.Fatalf("delta payload = %+v", d)
	}
}

func TestSendMessageFailureStoresNothing(t *testing.T) {
	fake := new_fake_vision_server(t, photo_rule)
	fake.status = 401
	log := &event_log{}
	svc := new_chat_service(t, fake.server.URL+"/v1", log)
	session, _ := svc.CreateSession("chat", Scope{})
	if _, err := svc.SendMessage(context.Background(), SendMessageRequest{SessionID: session.ID, Text: "hi"}); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid API key error, got %v", err)
	}
	stored, _ := svc.GetSession(session.ID)
	if len(stored.Messages) != 0 {
		t.Fatalf("failed send must not store messages")
	}
	if len(log.names(EventMessageDone)) != 0 {
		t.Fatalf("failed send must not emit done")
	}
}

func TestSendMessageValidatesInput(t *testing.T) {
	log := &event_log{}
	svc := new_chat_service(t, "http://127.0.0.1:1/v1", log)
	session, _ := svc.CreateSession("chat", Scope{})
	if _, err := svc.SendMessage(context.Background(), SendMessageRequest{SessionID: session.ID, Text: "   "}); err == nil {
		t.Fatalf("blank message must be refused")
	}
	long := strings.Repeat("a", max_message_runes+1)
	if _, err := svc.SendMessage(context.Background(), SendMessageRequest{SessionID: session.ID, Text: long}); err == nil {
		t.Fatalf("overlong message must be refused")
	}
}

func TestTestConnectionReportsProviderFailureAsResult(t *testing.T) {
	fake := new_fake_vision_server(t, photo_rule)
	fake.status = 401
	log := &event_log{}
	svc := new_chat_service(t, fake.server.URL+"/v1", log)
	models, _ := svc.ListModels()
	result, err := svc.TestConnection(context.Background(), models[0].ID, ModelInput{})
	if err != nil {
		t.Fatalf("test connection should not return a Go error for provider failures: %v", err)
	}
	if result.OK || !strings.Contains(result.Message, "invalid API key") {
		t.Fatalf("result = %+v", result)
	}

	fake.status = 0
	result, err = svc.TestConnection(context.Background(), "", ModelInput{
		DisplayName: "Draft",
		Provider:    ProviderCustom,
		BaseURL:     fake.server.URL + "/v1",
		Model:       "draft",
	})
	if err != nil || !result.OK {
		t.Fatalf("draft test should succeed, got %+v, %v", result, err)
	}
}
