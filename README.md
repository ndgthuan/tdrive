# TDrive

**A calm, private file drive that lives inside your Telegram account.**

TDrive turns a private Telegram channel into a folder tree you can browse, search, stream, share and mount. Files stay in Telegram; the app brings the organizing, previewing and AI-assisted tidying on top.

![License: MIT](https://img.shields.io/badge/license-MIT-0e6ba8?style=flat-square)
![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)
![Wails v3](https://img.shields.io/badge/Wails-v3%20beta-DF0000?style=flat-square)
![Svelte 5](https://img.shields.io/badge/Svelte-5-FF3E00?style=flat-square&logo=svelte&logoColor=white)

![TDrive library view](docs/screenshots/01_library.png)

> **Status:** This fork is in active development. The desktop app is the focus. The AI assistant and the link downloader are new and lightly tested. Mobile builds are inherited from upstream TDrive and are not tested in this fork yet.

## Download

Ready-made builds are on the [**Releases page**](https://github.com/ndgthuan/tdrive/releases/latest). Open the newest release, scroll to **Assets**, and pick the file for your device. You do not need to build anything.

| Device | File to download | Notes |
|---|---|---|
| Windows 10/11 (64-bit) | `TDrive-<version>-windows-amd64-setup.exe` | Installer. A portable `...-windows-amd64.zip` is next to it. |
| macOS (Apple silicon) | `TDrive-<version>-macos-arm64.zip` | Unzip and drag TDrive to Applications. See the first-launch note below. |
| Linux (64-bit) | `TDrive-<version>-x86_64.AppImage` | `chmod +x` it, then run it. Needs glibc 2.35 or newer. |
| Android 7.0 or newer (ARM64) | `TDrive-<version>-android-arm64.apk` | Allow installs from your browser or file manager when Android asks. |
| iPhone / iPad | `TDrive-<version>-ios-unsigned.ipa` | Not signed. Install with TrollStore, Sideloadly or AltStore. |
| Command line | `...-cli.zip` / `...-cli.tar.gz` | The `tdrive` CLI for scripting. |

Every release also ships `checksums.txt` and a signed `checksums.txt.sig`, so you can verify what you downloaded.

**macOS first launch.** The app is not notarized. If macOS says it cannot verify the developer, open **System Settings > Privacy & Security** and choose **Open Anyway**.

**Android.** The APK is signed with this fork's own key, so it cannot update over the original TDrive. Uninstall the original first if you have it.

**iOS.** There is no App Store or TestFlight build. The IPA is for sideloading only.

Prefer to compile it yourself? See [Run it](#run-it).

## What's in it

### Files and folders

- A private **My Drive** and collaborative **shared drives**, each backed by Telegram.
- Nested folders with rename, move, drag and drop, search, and a Trash that can be restored or emptied.
- Multi-select with checkboxes on desktop, or press and hold a row on touch devices.
- Keyboard navigation through the file grid, with a row action menu on F2, Delete or Shift + F10.

### Media

- Photo gallery across the whole drive, plus previews for images, PDF, text, code, audio and video.
- Streaming straight from Telegram, including encrypted media on personal drives.
- Audio-track, subtitle, speed and fit controls for video.

### AI assistant

- A side panel with two modes: **Agent** can act on your drive, **Ask** only answers.
- Pick a model from Gemini, Claude, OpenAI, OpenRouter or any OpenAI-compatible endpoint. API keys are kept in the operating system's credential store (Windows Credential Manager on Windows).
- Agent actions include listing and searching files, creating folders, renaming, moving, trashing, restoring and emptying the Trash.
- Every change shows an approval card first and can be reverted with **Undo**.
- Photo and video organizing builds a plan, shows a preview, and applies it only after you confirm.

### Sources and downloads

- **Sources** follow public Telegram channels as read-only libraries, in grid or list view, with thumbnails and download.
- **Download from link** uses yt-dlp: pick a quality preset (best, 1080p, 720p, 480p or MP3 audio), choose a destination drive and folder, and the file is uploaded straight to Telegram.

### Look and feel

- A true-black interface with one muted accent and [Be Vietnam Pro](https://fonts.google.com/specimen/Be+Vietnam+Pro) as the UI font.
- Light and dark themes.
- A layout that stays usable in narrow windows.
- A Windows drive mount that appears in Explorer as **Telegram Drive**.

### Languages

The interface ships in 19 languages: English, Vietnamese, Simplified and Traditional Chinese, Japanese, Korean, Spanish, Brazilian Portuguese, French, German, Italian, Russian, Ukrainian, Polish, Dutch, Turkish, Indonesian, Thai and Hindi. Change it under **Settings > Language**.

## Screenshots

| | |
|---|---|
| ![My Drive](docs/screenshots/01_library.png) | ![Photos](docs/screenshots/02_photos.png) |
| **My Drive.** Folders and files in one list, with actions always visible. | **Photos.** A gallery grouped by month. |
| ![Sources](docs/screenshots/03_sources_grid.png) | ![AI agent](docs/screenshots/04_ai_agent.png) |
| **Sources.** A public Telegram channel as a read-only grid. | **AI agent.** Renames and moves with Undo, and asks before trashing. |
| ![AI models](docs/screenshots/05_ai_models.png) | ![Download from link](docs/screenshots/06_download_from_link.png) |
| **AI models.** Connect Gemini or Claude and pick a default. | **Download from link.** Pick quality and destination, then upload to Telegram. |
| ![Languages](docs/screenshots/07_settings_language.png) | ![Multi-select](docs/screenshots/08_select_many.png) |
| **Languages.** 19 interface languages, applied instantly. | **Multi-select.** Download, move or delete a batch at once. |

## Inherited from TDrive

These features come from the upstream project and still work in this fork:

- Client-side encryption (XChaCha20-Poly1305) for personal-drive contents, with a vault password.
- Encrypted backup of photos and videos from selected desktop folders or Android storage.
- Shared drives with instant-join or approval-required invite links.
- Files over 2 GB split across Telegram messages and presented as one file, with interrupted unencrypted uploads resumable.
- Archive import (`.zip`, `.tar`, `.tar.gz`) and recursive folder download.
- A `tdrive` command-line tool with a background daemon, for scripts and agents.
- Mounting a personal drive as a local WebDAV volume in Finder, Explorer or a Linux file manager.

## Run it

There are no public releases for this fork yet, so build it from source.

### 1. Get Telegram API credentials

1. Sign in at [my.telegram.org/apps](https://my.telegram.org/apps) and create an application to get an **API ID** and **API hash**. Telegram's [setup guide](https://core.telegram.org/api/obtaining_api_id) explains the steps.
2. Keep both values private. TDrive stores them locally; the project does not ship a shared credential.

### 2. Build

Requirements: the Go version in [go.mod](go.mod), Node.js 22 with npm, and the native dependencies for your platform. See the [build guide](build/README.md) and the [Wails installation guide](https://v3.wails.io/getting-started/installation/).

Run these from the repository root:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.22

npm --prefix frontend ci
npm --prefix frontend run build

# Desktop app, with hot reload
wails3 dev

# Desktop binary into build/bin
wails3 task build

# Command-line tool
go build -o build/bin/tdrive ./cmd/tdrive
```

Use `wails3 task package` for a platform package (`.app`, NSIS installer or AppImage).

### 3. First launch

1. Enter your API ID and API hash in the setup screen.
2. Sign in with your phone number, the login code, and your 2FA password if you have one.
3. Choose the Telegram channel that holds your existing My Drive, or create an empty one.
4. Upload a file, create a folder, or join a shared drive.

On a second device, sign in with the same account and pick the same channel. If encryption is enabled, unlock it with your existing vault password.

## How it works

- **My Drive** is a private Telegram channel that you own.
- **Shared drives** are Telegram megagroups. Members collaborate in them.
- File contents are sent as document messages. Files over 2 GB are split into immutable parts and joined again on download.
- Folder structure and changes are stored as small, readable metadata messages, for example `TDrive | type=rename | object=f:42 | name=new.png`. Older `TDX1|...` messages still read correctly.
- Local SQLite is a projection of that history. It can be rebuilt by replaying the Telegram messages, so losing the local cache does not lose the drive.
- Optional encryption happens on your device before anything is sent. Telegram only sees ciphertext for encrypted files.

Do not edit or delete `TDrive | ...` messages in the Telegram app. Doing so can desynchronize the drive. Permanent deletion is the only operation that removes them, and TDrive performs it for you.

## Safety

- TDrive is an independent project. It is not affiliated with or endorsed by Telegram.
- Use it according to Telegram's terms, and keep your own backups of anything that matters.
- Treat your API hash, login session, vault password and invite links like passwords. Redact them from logs and screenshots.
- Telegram availability, rate limits and account restrictions are outside the app's control.

## Project layout

```text
backend/        Go services: ai, mediadl (yt-dlp), mountdav, photobackup, sync, tgclient, crypto, ...
frontend/       Svelte 5 and TypeScript UI (src/ui, src/i18n, src/locales)
internal/app/   Wails application wiring
cmd/tdrive/     Command-line tool
cmd/tdrived/    Background daemon
build/          Wails task files and platform packaging
docs/           Architecture notes and screenshots
```

Architecture notes are in [docs/architecture](docs/architecture/README.md).

## Development

Backend and desktop:

```bash
wails3 task build
go build ./...
go vet ./...
```

Frontend, from `frontend/`:

```bash
npm run typecheck
npm run test
npm run build
```

Run Go tests by package, not all at once. On Windows, do not run `go test ./...` wholesale: `backend/mountos` tests touch the live mount. Target the packages you changed, for example:

```bash
go test ./backend/ai/...
go test ./backend/mediadl/...
```

Browser end-to-end tests use `npm run test:e2e` from `frontend/`. Contribution and testing notes are in [CONTRIBUTING.md](CONTRIBUTING.md) and [TESTING.md](TESTING.md).

## Credits

- **[jeetrex17 / Jeetraj](https://github.com/jeetrex17/TDrive)** created the original TDrive, which this fork is built on. Thank you for the foundation.
- [Wails](https://wails.io) for the desktop shell.
- [gotd/td](https://github.com/gotd/td) for the Telegram client library.
- [Phosphor Icons](https://phosphoricons.com) for the interface icons.
- [Be Vietnam Pro](https://fonts.google.com/specimen/Be+Vietnam+Pro) for the UI typeface.
- [LobeHub icons](https://github.com/lobehub/lobe-icons) (MIT) for the AI provider logos.

## License

MIT. See [LICENSE](LICENSE). The original TDrive copyright notice is retained.
