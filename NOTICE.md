# TDrive Fork - Notice

## About this fork

TDrive Fork is a modified version of TDrive, a desktop drive that uses a private
Telegram channel as cloud storage. This fork adds its own features (for example
the AI assistant, the media downloader, transfer and settings improvements) and
attribution in the app, the CLI and the daemon log.

Author: Nguyễn Dương Gia Thuận (GitHub: ndgthuan)
Repository: https://github.com/ndgthuan/tdrive

Based on TDrive by Jeetraj (https://github.com/jeetrex17/TDrive), MIT.

## License

This fork is distributed under the MIT License. See the LICENSE file.

- Copyright (c) 2026 Jeetraj (original work, MIT License, unchanged)
- Copyright (c) 2026 Nguyễn Dương Gia Thuận (modifications and additions in this fork)

The MIT License requires the copyright notice and the permission notice to be
included in copies or substantial portions of the Software. Keep them intact
when you redistribute this code.

## Third-party components

Components listed below are the ones this repository uses or bundles. Their
license texts are available in their upstream packages.

Go modules (backend and CLI):

- Wails v3 (github.com/wailsapp/wails/v3), MIT - application framework
- gotd/td (github.com/gotd/td), MIT - Telegram MTProto client
- mp4ff (github.com/Eyevinn/mp4ff), MIT - MP4 parsing
- go-winio (github.com/Microsoft/go-winio), MIT
- wincred (github.com/danieljoos/wincred), MIT
- go-keyring (github.com/zalando/go-keyring), MIT
- google/uuid (github.com/google/uuid), BSD-3-Clause
- golang.org/x/crypto, x/image, x/net, x/sync, x/sys, x/term, x/text, BSD-3-Clause
- modernc.org/sqlite, BSD-3-Clause

Frontend packages (bundled into the app):

- phosphor-svelte 3.1.1 and Phosphor Icons, MIT - interface icons
- @lobehub/icons-static-svg logos (vendored in frontend/src/assets/brands),
  MIT (Lobe Icons). The Google, Anthropic, OpenAI and OpenRouter logos are
  trademarks of their owners and are used only to identify the provider.
- @fontsource/be-vietnam-pro, SIL Open Font License 1.1 - interface font
- Nunito (vendored in frontend/src/assets/fonts), SIL Open Font License 1.1
- @wailsio/runtime, MIT
- markdown-it, MIT
- libphonenumber-js, MIT
- highlight.js, BSD-3-Clause
- hls.js, Apache-2.0
- pdfjs-dist, Apache-2.0
- DOMPurify, dual-licensed MPL-2.0 or Apache-2.0

Tools downloaded at runtime (not bundled in this repository):

- yt-dlp, Unlicense - downloaded from the yt-dlp GitHub releases for the media downloader
- FFmpeg (GPL build from the yt-dlp FFmpeg-Builds releases), GPL - used for media processing

## Trademarks

Telegram is a trademark of Telegram FZ-LLC. This project is not affiliated with
or endorsed by Telegram. Other product names and logos belong to their owners.

## Disclaimer

The software is provided "AS IS", without warranty of any kind, as stated in the
MIT License.
