import { describe, expect, it } from 'vitest';
import { bind_no_save_guards } from './view_only_guards';

describe('bind_no_save_guards', () => {
    it('blocks save gestures only while the content is no-save', () => {
        const host = document.createElement('div');
        document.body.append(host);
        let no_save = true;
        const unbind = bind_no_save_guards(host, () => no_save);
        const fire = (event: Event): boolean => {
            host.dispatchEvent(event);
            return event.defaultPrevented;
        };
        const cancelable = { bubbles: true, cancelable: true };

        expect(fire(new Event('contextmenu', cancelable))).toBe(true);
        expect(fire(new Event('dragstart', cancelable))).toBe(true);
        expect(fire(new KeyboardEvent('keydown', { key: 's', ctrlKey: true, ...cancelable }))).toBe(true);
        expect(fire(new KeyboardEvent('keydown', { key: 'p', metaKey: true, ...cancelable }))).toBe(true);
        // Copy and select-all still work on read-only text.
        expect(fire(new KeyboardEvent('keydown', { key: 'c', ctrlKey: true, ...cancelable }))).toBe(false);

        no_save = false;
        expect(fire(new Event('contextmenu', cancelable))).toBe(false);
        expect(fire(new KeyboardEvent('keydown', { key: 's', ctrlKey: true, ...cancelable }))).toBe(false);

        no_save = true;
        unbind();
        expect(fire(new Event('contextmenu', cancelable))).toBe(false);
        host.remove();
    });
});
