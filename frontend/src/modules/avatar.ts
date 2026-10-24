// Avatar view model. Resolves to a profile photo when available, otherwise
// neutral initials. The fallback carries no hue: the CSS paints it with the
// surface-3 step, the text token and a soft border. Pure data so Svelte
// components can render it declaratively.

export interface AvatarUser {
    photoBase64?: string;
    displayName?: string;
    username?: string;
    userId?: number;
}

export interface AvatarView {
    photoUrl?: string;
    initials?: string;
}

export function avatarViewFor(user: AvatarUser | null): AvatarView {
    if (!user) return {};

    const photo = String(user.photoBase64 || '').trim();
    if (photo) {
        return { photoUrl: `data:image/jpeg;base64,${photo}` };
    }
    return { initials: initialsFor(user) };
}

function initialsFor(user: AvatarUser): string {
    const name = String(user.displayName || '').trim();
    if (name && !name.startsWith('@')) {
        const parts = name.split(/\s+/).filter(Boolean);
        if (parts.length >= 2) {
            return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
        }
        return parts[0].slice(0, 2).toUpperCase();
    }
    const handle = String(user.username || '').trim();
    if (handle) return handle.slice(0, 2).toUpperCase();
    return '?';
}
