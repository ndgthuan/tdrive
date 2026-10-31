// Matches the flood-wait text the backend puts in discovery errors, such as
// "tgclient: flood wait: 23s" or "rpc error code 420: FLOOD_WAIT_23".
const FLOOD_WAIT_PATTERN = /flood[ _]wait[:\s_]+(\d+)/i;

/**
 * Returns the seconds Telegram asked us to wait when the error text reports a
 * flood wait with a duration, or null for any other error.
 */
export function flood_wait_seconds(text: string): number | null {
    const match = FLOOD_WAIT_PATTERN.exec(text);
    if (!match) return null;
    const seconds = Number(match[1]);
    return Number.isSafeInteger(seconds) && seconds > 0 ? seconds : null;
}
