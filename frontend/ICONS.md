# Icon mapping: Lucide to Phosphor

The app's icons come from **Phosphor Icons** through the `phosphor-svelte` package
(pinned to `3.1.1`, MIT, Svelte 5 components, one ES module per icon under
`phosphor-svelte/lib/<Name>Icon`). This file maps each former `@lucide/svelte`
icon to the Phosphor component that replaced it, so a reviewer can audit the swap.

## Conventions used by the migration

- Imports keep the local alias the file already used (for example
  `import TriangleAlertIcon from 'phosphor-svelte/lib/WarningIcon';`). Only the
  source path changed, so the alias name can differ from the component it now points to.
  The right-hand column below is the component actually imported.
- `strokeWidth` props were removed. Phosphor draws filled outlines and has no
  stroke-width prop. The weight is chosen instead:
  - `weight` omitted (regular) for everything at 15px and larger.
  - `weight="bold"` for literal sizes of 14px or less, where the old stroke was heavier.
  - `weight="fill"` where the old code passed `fill="currentColor"` (the Play glyph in `ChannelView.svelte`).
- Where a usage had no `size` prop, `size={24}` was added to keep Lucide's 24px default.
- Icons inherit `currentColor` (phosphor-svelte's default `color`).
- Variable sizes were left as they were. Two sites use a ternary size and therefore stay at regular weight:
  `ui/channels/ChannelSortMenu.svelte` (`ArrowDownUpIcon`, 22 or 14px) and
  `ui/notifications/ResumableDownloadRow.svelte` (`ArrowDownIcon`, 14 or 15px).

## Name mapping

| Lucide (`@lucide/svelte/icons/…`) | Phosphor (`phosphor-svelte/lib/…`) |
|---|---|
| `archive` | `ArchiveIcon` |
| `arrow-down` | `ArrowDownIcon` |
| `arrow-down-up` | `ArrowsDownUpIcon` |
| `arrow-right` | `ArrowRightIcon` |
| `arrow-up` | `ArrowUpIcon` |
| `audio-lines` | `WaveformIcon` |
| `ban` | `ProhibitIcon` |
| `bell` | `BellIcon` |
| `bot` | `RobotIcon` |
| `calendar` | `CalendarBlankIcon` |
| `captions` | `ClosedCaptioningIcon` |
| `check` | `CheckIcon` |
| `chevron-down` | `CaretDownIcon` |
| `chevron-left` | `CaretLeftIcon` |
| `chevron-right` | `CaretRightIcon` |
| `circle-alert` | `WarningCircleIcon` |
| `circle-check` | `CheckCircleIcon` |
| `circle-user` | `UserCircleIcon` |
| `circle-x` | `XCircleIcon` |
| `clock` | `ClockIcon` |
| `cloud` | `CloudIcon` |
| `cloud-alert` | `CloudWarningIcon` |
| `cloud-upload` | `CloudArrowUpIcon` |
| `code` | `CodeIcon` |
| `database` | `DatabaseIcon` |
| `download` | `DownloadSimpleIcon` |
| `eject` | `EjectIcon` |
| `ellipsis` | `DotsThreeIcon` |
| `external-link` | `ArrowSquareOutIcon` |
| `eye` | `EyeIcon` |
| `eye-off` | `EyeSlashIcon` |
| `file` | `FileIcon` |
| `file-text` | `FileTextIcon` |
| `file-up` | `FileArrowUpIcon` |
| `fingerprint` | `FingerprintIcon` |
| `folder` | `FolderIcon` |
| `folder-input` | `FolderOpenIcon` (move-into action; no Phosphor folder-with-arrow glyph exists) |
| `folder-plus` | `FolderPlusIcon` |
| `folder-up` | `UploadSimpleIcon` (the "Upload folder" action; Phosphor has no folder-with-arrow glyph) |
| `hard-drive` | `HardDriveIcon` |
| `hash` | `HashIcon` |
| `image` | `ImageIcon` |
| `image-off` | `ImageBrokenIcon` |
| `images` | `ImagesIcon` |
| `info` | `InfoIcon` |
| `inbox` | `TrayIcon` |
| `key-round` | `KeyIcon` |
| `layers` | `StackIcon` |
| `link` | `LinkIcon` |
| `link-2` | `LinkSimpleIcon` |
| `list-video` | `PlaylistIcon` |
| `loader-circle` | `CircleNotchIcon` (spinner; rotated by the app's CSS) |
| `lock` | `LockIcon` |
| `lock-keyhole` | `LockKeyIcon` |
| `log-out` | `SignOutIcon` |
| `mail` | `EnvelopeIcon` |
| `maximize` | `ArrowsOutIcon` |
| `minimize` | `ArrowsInIcon` |
| `moon` | `MoonIcon` |
| `music` | `MusicNoteIcon` |
| `music-2` | `MusicNotesIcon` |
| `palette` | `PaletteIcon` |
| `pause` | `PauseIcon` |
| `pencil` | `PencilSimpleIcon` |
| `play` | `PlayIcon` (weight `fill` where the old code filled it) |
| `plus` | `PlusIcon` |
| `radio-tower` | `BroadcastIcon` |
| `refresh-cw` | `ArrowsClockwiseIcon` |
| `rotate-ccw` | `ArrowCounterClockwiseIcon` |
| `rotate-cw` | `ArrowClockwiseIcon` |
| `search` | `MagnifyingGlassIcon` |
| `settings` | `GearIcon` |
| `share-2` | `ShareNetworkIcon` |
| `sun` | `SunIcon` |
| `table` | `TableIcon` |
| `text-align-start` | `TextAlignLeftIcon` |
| `trash-2` | `TrashIcon` |
| `triangle-alert` | `WarningIcon` |
| `upload` | `UploadSimpleIcon` |
| `user-round` | `UserIcon` |
| `users-round` | `UsersThreeIcon` |
| `video` | `VideoCameraIcon` |
| `volume-2` | `SpeakerHighIcon` |
| `volume-x` | `SpeakerXIcon` |
| `x` | `XIcon` |

## Other notes

- The `glyph` strings in `ui/mobile/item-state.ts` (for example `'circle-check'`, `'triangle-alert'`)
  are internal keys, not package imports. They are still named after the old Lucide glyphs and were kept
  as they are, and the `GLYPHS` map in `ui/mobile/ItemStatus.svelte` maps them to the new components.
- The video skip-10 controls in `ui/video/VideoModal.svelte` remain hand-drawn SVG, as before
  (see `ui/icon-system.test.ts`).
