// Typed boundary for the AI model profiles. Keep the exported names stable:
// the AI panel and the settings dialog both import them.
import {
    AddModel,
    DeleteModel,
    ListModels,
    SetDefaultModel,
    TestConnection,
    UpdateModel,
} from '../../bindings/TDrive/internal/app/aiservice';
import type { ModelInput, ModelView, TestResult } from '../../bindings/TDrive/backend/ai/models';
import { invokeBackend } from './gateway';

export { Provider } from '../../bindings/TDrive/backend/ai/models';
export type { ModelInput, ModelView, TestResult };

/** Configured model profiles. API keys never come back; has_api_key says whether one is stored. */
export async function list_models(): Promise<ModelView[]> {
    const models = await invokeBackend(ListModels);
    return models ?? [];
}

/** Stores a profile. The key goes to the OS credential store. */
export function add_model(input: ModelInput): Promise<ModelView> {
    return invokeBackend(AddModel, input);
}

/** Changes a profile. An empty api_key keeps the stored key; clear_api_key removes it. */
export function update_model(id: string, input: ModelInput): Promise<ModelView> {
    return invokeBackend(UpdateModel, id, input);
}

/** Removes a profile and its stored key. */
export function delete_model(id: string): Promise<void> {
    return invokeBackend(DeleteModel, id);
}

/** Makes a profile the default for chat and organize. */
export function set_default_model(id: string): Promise<void> {
    return invokeBackend(SetDefaultModel, id);
}

/**
 * Sends a tiny request. Pass model_id to test a stored profile (draft fields
 * override it), or '' to test the draft alone. Provider failures come back as
 * ok=false, not as a rejected promise.
 */
export function test_connection(model_id: string, draft: ModelInput): Promise<TestResult> {
    return invokeBackend(TestConnection, model_id, draft);
}
