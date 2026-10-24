// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Maps a model provider to its official logo. Monochrome logos inherit the
// text colour; Gemini and Claude keep their own colours. Custom endpoints have
// no brand, so they get a neutral icon instead (see brand_icon.svelte).
import { Provider } from '../../../bindings/TDrive/backend/ai/models';
import gemini_svg from '../../assets/brands/gemini.svg?raw';
import claude_svg from '../../assets/brands/claude.svg?raw';
import openai_svg from '../../assets/brands/openai.svg?raw';
import openrouter_svg from '../../assets/brands/openrouter.svg?raw';

const BRAND_SVG: Partial<Record<Provider, string>> = {
    [Provider.ProviderGemini]: gemini_svg,
    [Provider.ProviderAnthropic]: claude_svg,
    [Provider.ProviderOpenAI]: openai_svg,
    [Provider.ProviderOpenRouter]: openrouter_svg,
};

/** The logo markup for a provider, or null when it has none (custom endpoints). */
export function brand_svg(provider: Provider): string | null {
    return BRAND_SVG[provider] ?? null;
}
