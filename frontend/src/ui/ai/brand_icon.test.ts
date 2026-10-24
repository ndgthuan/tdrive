// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { describe, expect, it } from 'vitest';
import { Provider } from '../../api/ai';
import { brand_svg } from './brand_icon';

describe('brand logos', () => {
    it('uses the official logo for each named provider', () => {
        expect(brand_svg(Provider.ProviderGemini)).toContain('<title>Gemini</title>');
        expect(brand_svg(Provider.ProviderAnthropic)).toContain('<title>Claude</title>');
        expect(brand_svg(Provider.ProviderOpenAI)).toContain('<title>OpenAI</title>');
        expect(brand_svg(Provider.ProviderOpenRouter)).toContain('<title>OpenRouter</title>');
    });

    it('keeps the single-colour marks on the text colour', () => {
        expect(brand_svg(Provider.ProviderOpenAI)).toContain('fill="currentColor"');
        expect(brand_svg(Provider.ProviderOpenRouter)).toContain('fill="currentColor"');
    });

    it('gives custom endpoints no brand, so the icon falls back to a neutral plug', () => {
        expect(brand_svg(Provider.ProviderCustom)).toBeNull();
    });
});
