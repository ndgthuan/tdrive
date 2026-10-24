import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import ChannelViewToggle from './channel_view_toggle.svelte';

let app: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

afterEach(async () => {
    if (app) await unmount(app);
    host?.remove();
    app = null;
    host = null;
});

function render(value: 'grid' | 'list', onChange = vi.fn()) {
    host = document.createElement('div');
    document.body.append(host);
    app = mount(ChannelViewToggle, { target: host, props: { value, onChange } });
    flushSync();
    return onChange;
}

function button(label: string): HTMLButtonElement {
    const found = host?.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
    if (!found) throw new Error(`no ${label} button`);
    return found;
}

describe('ChannelViewToggle', () => {
    it('offers Grid and List as two labelled toggle buttons', () => {
        render('grid');
        const group = host?.querySelector('[role="group"]');
        expect(group?.getAttribute('aria-label')).toBe('Show as');
        expect(host?.querySelectorAll('button')).toHaveLength(2);
        expect(button('Grid').title).toBe('Grid');
        expect(button('List').title).toBe('List');
    });

    it('marks the current layout as pressed', () => {
        render('list');
        expect(button('List').getAttribute('aria-pressed')).toBe('true');
        expect(button('Grid').getAttribute('aria-pressed')).toBe('false');
    });

    it('reports the layout it is switched to', () => {
        const onChange = render('grid');
        button('List').click();
        expect(onChange).toHaveBeenCalledWith('list');
        button('Grid').click();
        expect(onChange).toHaveBeenLastCalledWith('grid');
    });
});
