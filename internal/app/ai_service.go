package app

import (
	"context"

	"TDrive/backend/ai"
)

// AIService is the Wails-bound face of the AI assistant. It holds model
// profiles, chat sessions and the photo organize flow. The domain logic lives
// in backend/ai; this type only supplies the engine-backed drive and the event
// emitter, and waits for startup where a drive is needed.
//
// Model profile management needs no drive, so it works before a drive is
// selected. Organize and chat go through the normal startup gate.
type AIService struct {
	host serviceHost
	core *ai.Service
}

func new_ai_service(host serviceHost) *AIService {
	service := &AIService{host: host}
	service.core = ai.NewService(ai.Options{
		Drive: service.drive,
		Emit: func(name string, payload any) {
			host.emit(name, payload)
		},
	})
	return service
}

// ListModels returns the configured model profiles. API keys are never included;
// has_api_key says whether one is stored.
func (s *AIService) ListModels() ([]ai.ModelView, error) {
	return s.core.ListModels()
}

// AddModel stores a new model profile. The API key goes to the OS credential
// store and is not returned.
func (s *AIService) AddModel(input ai.ModelInput) (ai.ModelView, error) {
	return s.core.AddModel(input)
}

// UpdateModel changes a profile. An empty api_key keeps the stored key;
// clear_api_key removes it.
func (s *AIService) UpdateModel(id string, input ai.ModelInput) (ai.ModelView, error) {
	return s.core.UpdateModel(id, input)
}

// DeleteModel removes a profile and its stored key.
func (s *AIService) DeleteModel(id string) error {
	return s.core.DeleteModel(id)
}

// SetDefaultModel makes a profile the default for chat and organize.
func (s *AIService) SetDefaultModel(id string) error {
	return s.core.SetDefaultModel(id)
}

// TestConnection sends a tiny request to a model. Pass model_id to test a stored
// profile (draft fields override it), or an empty model_id to test a draft alone.
// Provider failures come back as ok=false with a message, not as an error.
func (s *AIService) TestConnection(ctx context.Context, model_id string, draft ai.ModelInput) (ai.TestResult, error) {
	return s.core.TestConnection(ctx, model_id, draft)
}

// CreateSession starts an empty chat in the given drive and folder.
func (s *AIService) CreateSession(title string, scope ai.Scope) (ai.ChatSession, error) {
	return s.core.CreateSession(title, scope)
}

// ListSessions returns chat summaries, most recent first.
func (s *AIService) ListSessions() ([]ai.SessionSummary, error) {
	return s.core.ListSessions()
}

// GetSession returns one chat with its messages.
func (s *AIService) GetSession(id string) (ai.ChatSession, error) {
	return s.core.GetSession(id)
}

// DeleteSession removes a chat. Drive files are not touched.
func (s *AIService) DeleteSession(id string) error {
	return s.core.DeleteSession(id)
}

// SendMessage sends one user turn and returns the assistant reply. The call
// blocks until the model answers.
func (s *AIService) SendMessage(ctx context.Context, request ai.SendMessageRequest) (ai.ChatMessage, error) {
	return s.core.SendMessage(ctx, request)
}

// ResolveAction answers an ai:action_request raised by a tool that needs the
// user's approval (delete, bulk move, folder rename). approved=false denies it.
// Each request id works once; the agent treats a missing answer as denied after
// five minutes.
func (s *AIService) ResolveAction(request_id string, approved bool) error {
	return s.core.ResolveAction(request_id, approved)
}

// UndoAction reverses an agent action by its undo_token: a rename, move, folder
// creation or trash. Organize apply tokens work here too. The token works once.
// Failures come back as ok=false with a message.
func (s *AIService) UndoAction(ctx context.Context, undo_token string) (ai.UndoActionResult, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return ai.UndoActionResult{}, err
	}
	return s.core.UndoAction(ctx, undo_token)
}

// PlanOrganize analyzes the photos in the scope and returns a plan for review.
// Progress arrives as ai:organize_progress events. Nothing moves yet.
func (s *AIService) PlanOrganize(ctx context.Context, request ai.PlanRequest) (ai.Plan, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return ai.Plan{}, err
	}
	return s.core.PlanOrganize(ctx, request)
}

// ApplyPlan moves the planned files. excluded_file_ids are removed from the plan
// first. Files are only moved, never deleted. Progress arrives as
// ai:apply_progress events.
func (s *AIService) ApplyPlan(ctx context.Context, plan_id string, excluded_file_ids []int64) (ai.ApplyResult, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return ai.ApplyResult{}, err
	}
	return s.core.ApplyPlan(ctx, plan_id, excluded_file_ids)
}

// UndoApply moves files applied under undo_token back to their original folders.
// The token works once.
func (s *AIService) UndoApply(ctx context.Context, undo_token string) (ai.UndoResult, error) {
	if err := s.host.WaitForBackendReady(ctx); err != nil {
		return ai.UndoResult{}, err
	}
	return s.core.UndoApply(ctx, undo_token)
}

// drive resolves the engine-backed drive access used by organize. It fails with
// the standard backend-not-ready error until startup has built the engine.
func (s *AIService) drive() (ai.Drive, error) {
	engine := s.host.coreEngine()
	if engine == nil {
		return nil, errBackendUnavailable
	}
	files, err := engineFileService(engine)
	if err != nil {
		return nil, err
	}
	reads, err := engineReadService(engine)
	if err != nil {
		return nil, err
	}
	folders := engine.FolderService()
	if folders == nil {
		return nil, errBackendUnavailable
	}
	return &aiDrive{files: files, reads: reads, folders: folders}, nil
}
