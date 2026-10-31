// Matches a flood wait followed by a Go duration, such as
// "tgclient: flood wait: 1m31s", "flood wait: 5m0s", "flood wait: 1h1m40s" or
// "flood wait: 1.5s". Go's time.Duration.String never has spaces, so the
// duration is one run of number+unit pairs. The lookahead rejects a unit that
// is followed by more letters (for example "23 seconds").
const GO_DURATION_PATTERN = /flood[ _]wait[:\s_]+((?:\d+(?:\.\d+)?(?:ms|h|m|s))+)(?![a-z])/i;

// Matches a flood wait followed by a bare seconds count, such as
// "flood wait: 23s" or "rpc error code 420: FLOOD_WAIT_30".
const SECONDS_PATTERN = /flood[ _]wait[:\s_]+(\d+)/i;

const DURATION_PART_PATTERN = /(\d+(?:\.\d+)?)(ms|h|m|s)/gi;

const UNIT_SECONDS: Record<string, number> = { ms: 0.001, s: 1, m: 60, h: 3600 };

// Converts the duration text to whole seconds, rounding any fraction up so the
// wait shown to the user is never shorter than the one Telegram asked for.
function duration_to_seconds(duration: string): number | null {
    let total = 0;
    for (const part of duration.matchAll(DURATION_PART_PATTERN)) {
        total += Number(part[1]) * UNIT_SECONDS[part[2].toLowerCase()];
    }
    const seconds = Math.ceil(total);
    return Number.isSafeInteger(seconds) && seconds > 0 ? seconds : null;
}

/**
 * Returns the whole seconds Telegram asked us to wait when the error text
 * reports a flood wait with a duration, or null for any other error.
 */
export function flood_wait_seconds(text: string): number | null {
    const duration = GO_DURATION_PATTERN.exec(text);
    if (duration) return duration_to_seconds(duration[1]);
    const seconds = SECONDS_PATTERN.exec(text);
    if (!seconds) return null;
    const value = Number(seconds[1]);
    return Number.isSafeInteger(value) && value > 0 ? value : null;
}
