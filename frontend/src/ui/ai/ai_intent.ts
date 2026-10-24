// Decides when a message is an organize request. Keywords only: anything
// ambiguous stays a normal chat turn.
const ORGANIZE_PATTERN = /(^|[^\p{L}])(organi[sz]e|organi[sz]ing|sort|sorting|arrange|sắp\s*xếp|phân\s*loại|gom)($|[^\p{L}])/iu;

export function is_organize_request(text: string): boolean {
    return ORGANIZE_PATTERN.test(text.trim());
}

/** Turns the first words of a message into a short session title. */
export function title_from_text(text: string): string {
    const single = text.replace(/\s+/g, ' ').trim();
    return single.length > 48 ? `${single.slice(0, 47).trimEnd()}…` : single;
}
