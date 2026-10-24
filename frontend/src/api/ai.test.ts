import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as aiBindings from '../../bindings/TDrive/internal/app/aiservice';
import { list_models, test_connection, type ModelInput } from './ai';

vi.mock('../../bindings/TDrive/internal/app/aiservice', () => ({
    AddModel: vi.fn(),
    DeleteModel: vi.fn(),
    ListModels: vi.fn(),
    SetDefaultModel: vi.fn(),
    TestConnection: vi.fn(),
    UpdateModel: vi.fn(),
}));

const draft: ModelInput = {
    display_name: 'Internal',
    provider: 'custom' as ModelInput['provider'],
    base_url: 'https://llm.internal/v1',
    model: 'my-model',
    vision_enabled: false,
    api_key: 'sk-test',
    clear_api_key: false,
};

describe('AI model api wrapper', () => {
    beforeEach(() => {
        vi.mocked(aiBindings.ListModels).mockReset();
        vi.mocked(aiBindings.TestConnection).mockReset();
    });

    it('returns an empty list when the backend has no models yet', async () => {
        vi.mocked(aiBindings.ListModels).mockResolvedValue(null as never);
        await expect(list_models()).resolves.toEqual([]);
    });

    it('passes the draft through to the connection test and returns its verdict', async () => {
        vi.mocked(aiBindings.TestConnection).mockResolvedValue({ ok: false, message: 'Invalid API key.' } as never);
        await expect(test_connection('', draft)).resolves.toEqual({ ok: false, message: 'Invalid API key.' });
        expect(aiBindings.TestConnection).toHaveBeenCalledWith('', draft);
    });

    it('surfaces a rejected binding as a backend error', async () => {
        vi.mocked(aiBindings.ListModels).mockRejectedValue(new Error('store locked'));
        await expect(list_models()).rejects.toThrow('store locked');
    });
});
