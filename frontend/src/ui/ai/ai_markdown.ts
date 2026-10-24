// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Renders assistant replies as sanitized HTML. Raw HTML in the model output is
// dropped and links stay plain text, so a reply cannot navigate the app.
import MarkdownIt from 'markdown-it';
import createDOMPurify from 'dompurify';

const markdown = new MarkdownIt({ html: false, linkify: false, typographer: false });

export function render_ai_markdown(source: string): string {
    const DOMPurify = createDOMPurify(window);
    return DOMPurify.sanitize(markdown.render(source), { USE_PROFILES: { html: true } });
}
