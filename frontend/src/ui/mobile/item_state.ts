import { t } from '../../i18n/i18n';

/**
 * Every offline/sync state a file can be in, and nothing else.
 *
 * This list is closed on purpose. The failure mode it exists to prevent is one
 * ambiguous cloud glyph standing for six different situations, where "is this
 * on my phone?", "is it still uploading?" and "did it break?" all look the
 * same and the only way to find out is to tap and wait. Each state below
 * answers one question the reader actually has, and every surface that shows
 * file status -- rows, the photo grid, the detail sheet, search results, the
 * transfer queue -- renders from this table so they cannot drift apart.
 *
 * Colour carries meaning here, so it is rationed to three roles:
 *   muted      nothing is happening and nothing is wrong
 *   accent     bytes are moving right now, and only then
 *   danger     a person has to decide something
 * `available-offline` is a confirmed state but deliberately not accent: if
 * accent marked both "working" and "finished", a glance could no longer tell
 * them apart, which is the whole job of the badge.
 */

export type ItemState =
    | 'online-only'
    | 'queued'
    | 'downloading'
    | 'available-offline'
    | 'syncing'
    | 'conflict'
    | 'failed';

export type ItemStateTone = 'muted' | 'accent' | 'danger';

export interface ItemStateDescriptor {
    /** Icon glyph name, resolved to a component by item_status.svelte. */
    readonly glyph: string;
    /** Spoken and written label. Short enough to sit in a row, plain enough to be believed. */
    readonly label: string;
    readonly tone: ItemStateTone;
    /** Whether the glyph turns. A directional download arrow must stay upright. */
    readonly spins: boolean;
    /**
     * Whether the row grows a second line to explain itself. Only the two
     * states a person must act on earn the extra height -- everything else
     * stays on the standard row so a list of ordinary files reads evenly.
     */
    readonly needsExplanation: boolean;
    /** Sentence shown on the explaining line and in the detail sheet. */
    readonly detail: string;
    /**
     * Whether this state is worth drawing a badge for.
     *
     * Only departures from the resting state earn a mark. Badging every row
     * with the state nearly every row is in is the same as labelling every
     * file "file": it costs a column and returns nothing, and it trains the
     * eye to skip the place where the real signals appear.
     *
     * `online-only` is that resting state today, because nothing can be kept
     * on the device yet. When pinning ships, some files will be local and some
     * will not, the distinction starts carrying information, and this flips to
     * true so online-only gets its cloud back.
     */
    readonly marked: boolean;
}

/** A state's table row. Text is stored as keys and resolved per call, so it follows the language. */
type ItemStateEntry = Omit<ItemStateDescriptor, 'label' | 'detail'> & {
    readonly label_key: string;
    readonly detail_key: string;
};

export const ITEM_STATES: Readonly<Record<ItemState, ItemStateEntry>> = {
    'online-only': {
        glyph: 'cloud',
        label_key: 'mobile.item_online_only',
        tone: 'muted',
        spins: false,
        needsExplanation: false,
        detail_key: 'mobile.item_online_only_detail',
        marked: false,
    },
    queued: {
        glyph: 'clock',
        label_key: 'mobile.item_queued',
        tone: 'muted',
        spins: false,
        needsExplanation: false,
        detail_key: 'mobile.item_queued_detail',
        marked: true,
    },
    downloading: {
        glyph: 'download',
        label_key: 'mobile.item_downloading',
        tone: 'accent',
        spins: false,
        needsExplanation: false,
        detail_key: 'mobile.item_downloading_detail',
        marked: true,
    },
    'available-offline': {
        glyph: 'circle-check',
        label_key: 'mobile.item_available_offline',
        tone: 'muted',
        spins: false,
        needsExplanation: false,
        detail_key: 'mobile.item_available_offline_detail',
        marked: true,
    },
    syncing: {
        glyph: 'refresh-cw',
        label_key: 'mobile.item_syncing',
        tone: 'accent',
        spins: true,
        needsExplanation: false,
        detail_key: 'mobile.item_syncing_detail',
        marked: true,
    },
    conflict: {
        glyph: 'triangle-alert',
        label_key: 'mobile.item_conflict',
        tone: 'danger',
        spins: false,
        needsExplanation: true,
        detail_key: 'mobile.item_conflict_detail',
        marked: true,
    },
    failed: {
        glyph: 'circle-alert',
        label_key: 'mobile.item_failed',
        tone: 'danger',
        spins: false,
        needsExplanation: true,
        detail_key: 'mobile.item_failed_detail',
        marked: true,
    },
};

/** Every state, in the order a file usually travels through them. */
export const ITEM_STATE_ORDER: readonly ItemState[] = [
    'online-only',
    'queued',
    'downloading',
    'available-offline',
    'syncing',
    'conflict',
    'failed',
];

/** The state's glyph, tone and flags, with its label and detail resolved in the current language. */
export function itemStateDescriptor(state: ItemState): ItemStateDescriptor {
    const entry = ITEM_STATES[state];
    return { ...entry, label: t(entry.label_key), detail: t(entry.detail_key) };
}

/**
 * True when the badge should take a tap and open the transfer queue. Only the
 * states the queue can say more about are worth the trip: a tap that opens an
 * empty or unrelated screen is worse than a badge that does nothing.
 */
export function opensQueue(state: ItemState): boolean {
    return state === 'queued' || state === 'downloading' || state === 'syncing' || state === 'failed';
}

/**
 * The state a file is in, given what the projection knows and what the
 * transfer queue is doing with it. Transfers win over stored state: a file
 * that is on the device *and* being re-uploaded should read "syncing", because
 * that is the part that is still in motion and could still go wrong.
 */
export interface ItemStateInput {
    /** A transfer touching this file right now, if any. */
    readonly transfer?: { status: 'queued' | 'active' | 'paused' | 'failed' | 'done'; direction: 'up' | 'down' } | undefined;
    /** The projection says the local copy diverged from the vault. */
    readonly conflicted?: boolean;
    /** A complete local copy exists and is pinned to stay. */
    readonly offline?: boolean;
}

export function resolveItemState(input: ItemStateInput): ItemState {
    // A conflict outranks everything: no amount of transfer progress makes the
    // question of which version to keep go away, and quietly syncing over it
    // would be the one outcome the user cannot undo.
    if (input.conflicted) return 'conflict';

    const transfer = input.transfer;
    if (transfer && transfer.status !== 'done') {
        if (transfer.status === 'failed') return 'failed';
        // Paused reads as queued rather than as its own badge: from the row's
        // point of view both mean "not moving, will move later", and the
        // transfer queue is where the reason belongs.
        if (transfer.status === 'queued' || transfer.status === 'paused') return 'queued';
        return transfer.direction === 'up' ? 'syncing' : 'downloading';
    }

    return input.offline ? 'available-offline' : 'online-only';
}
