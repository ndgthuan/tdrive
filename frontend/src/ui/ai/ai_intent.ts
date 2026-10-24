// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Turns the first words of a message into a short session title. Deciding
// whether a message needs a tool is left to the model in Agent mode.
export function title_from_text(text: string): string {
    const single = text.replace(/\s+/g, ' ').trim();
    return single.length > 48 ? `${single.slice(0, 47).trimEnd()}…` : single;
}
