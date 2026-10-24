import { AsYouType, getCountries, getCountryCallingCode, parsePhoneNumberFromString, type CountryCode } from 'libphonenumber-js/min';

import { t } from '../../i18n/i18n';

export interface CountryOption {
    code: CountryCode;
    name: string;
    callingCode: string;
    flag: string;
}

/** Every country, named and sorted in the given locale (e.g. 'en' or 'vi'). */
export function countries_for(locale: string): CountryOption[] {
    const names = new Intl.DisplayNames([locale], { type: 'region' });
    return getCountries().map(code => ({
        code,
        name: names.of(code) ?? code,
        callingCode: getCountryCallingCode(code),
        flag: [...code].map(letter => String.fromCodePoint(127397 + letter.charCodeAt(0))).join(''),
    })).sort((a, b) => a.name.localeCompare(b.name, locale));
}

export const countries = countries_for('en');

export function countryForInternationalNumber(raw: string): CountryCode | undefined {
    if (!raw.trim().startsWith('+')) return undefined;
    return parsePhoneNumberFromString(raw, { extract: false })?.country;
}

export function nationalNumberForCountryChange(raw: string): string {
    if (!raw.trim().startsWith('+')) return raw;
    const parser = new AsYouType();
    parser.input(raw);
    const callingCode = parser.getCallingCode();
    return callingCode ? parser.getChars().slice(1 + callingCode.length) : '';
}

// Telegram remains authoritative about account eligibility, including Fragment numbers.
// Use metadata to handle national prefixes, with a syntax-only fallback for
// international numbers absent from current allocation metadata.
export function normalizePhoneNumber(raw: string, country?: CountryCode): { number: string } | { error: string } {
    const input = raw.trim();
    if (!input) return { error: t('auth.phone_err_empty') };
    if (!/^\+?[\d\s().-]+$/.test(input)) {
        return { error: t('auth.phone_err_chars') };
    }
    const digits = input.replace(/\D/g, '');
    if (input.startsWith('+')) {
        if (!/^[1-9]\d{3,14}$/.test(digits)) return { error: t('auth.phone_err_intl') };
        const parsed = parsePhoneNumberFromString(input, { extract: false });
        return { number: parsed?.countryCallingCode !== '888' && parsed?.isPossible()
            ? parsed.number : `+${digits}` };
    }
    if (!country) return { error: t('auth.phone_err_no_country') };
    const parsed = parsePhoneNumberFromString(input, { defaultCountry: country, extract: false });
    if (!parsed || !parsed.isPossible()) return { error: t('auth.phone_err_invalid') };
    return { number: parsed.number };
}
