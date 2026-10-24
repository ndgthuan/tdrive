# Copyright (c) 2026 Nguyễn Dương Gia Thuận
"""TDrive's entry point into yt-dlp on iOS, run by the embedded Python.

Staged next to yt-dlp in the app bundle and called from TDriveMediaDl.inc on a
background thread. Every function takes the request id the Go side issued, so
output lines and cancels always land on the right download.

The native side passes a callback taking (request_id, line). yt-dlp writes its
output to sys.stdout and sys.stderr; both are routed through one sink that
splits it into lines.
"""

import io
import sys
import threading

_state_lock = threading.Lock()
_cancelled = set()

# The yt-dlp this build ships. It is pinned in build/ios/Taskfile.yml and only
# changes with an app update, because the app cannot install packages itself.
BUNDLED_VERSION_FALLBACK = "2026.8.19"


class _LineSink(io.TextIOBase):
    """Splits yt-dlp output on \\n and \\r and reports each non-empty line."""

    def __init__(self, request_id, on_line):
        self._request_id = request_id
        self._on_line = on_line
        self._pending = ""

    def writable(self):
        return True

    def write(self, text):
        if not text:
            return 0
        if _is_cancelled(self._request_id):
            # Best effort: yt-dlp writes on every progress tick, so the next
            # write after a cancel stops the download.
            raise InterruptedError("download cancelled")
        self._pending += text
        while True:
            cut = _first_break(self._pending)
            if cut < 0:
                break
            line = self._pending[:cut].strip()
            self._pending = self._pending[cut + 1 :]
            if line:
                self._on_line(self._request_id, line)
        return len(text)

    def flush(self):
        line = self._pending.strip()
        self._pending = ""
        if line:
            self._on_line(self._request_id, line)


def _first_break(text):
    indexes = [i for i in (text.find("\n"), text.find("\r")) if i >= 0]
    return min(indexes) if indexes else -1


def _is_cancelled(request_id):
    with _state_lock:
        return request_id in _cancelled


def cancel(request_id):
    """Marks a job so its next output write stops it."""
    with _state_lock:
        _cancelled.add(request_id)


def status():
    """Reports the bundled yt-dlp. Never runs a download."""
    try:
        import yt_dlp.version

        version = yt_dlp.version.__version__
    except Exception:
        return {"installed": False, "version": "", "ffmpeg": False}
    # iOS has no ffmpeg in this build, so merged and audio-extracted formats
    # are not offered; the Go side reads ffmpeg=false and avoids them.
    return {"installed": True, "version": version or BUNDLED_VERSION_FALLBACK, "ffmpeg": False}


def run(request_id, argv, on_line):
    """Runs yt-dlp with argv and returns its exit code (0 on success)."""
    import yt_dlp

    sink = _LineSink(request_id, on_line)
    saved_out, saved_err = sys.stdout, sys.stderr
    sys.stdout = sys.stderr = sink
    code = 0
    try:
        yt_dlp.main(list(argv))
    except SystemExit as request:
        code = _exit_code(request.code)
    except InterruptedError:
        code = 130
    finally:
        sys.stdout, sys.stderr = saved_out, saved_err
        sink.flush()
        with _state_lock:
            _cancelled.discard(request_id)
    return code


def _exit_code(value):
    if value is None:
        return 0
    if isinstance(value, int):
        return value
    return 1
