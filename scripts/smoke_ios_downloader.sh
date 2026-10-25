#!/usr/bin/env bash
# Exercise the actual embedded iOS interpreter and download bytes on Simulator.
set -euo pipefail
workspace="$(pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/tdrive-python-smoke.XXXXXX")"
server_pid=""
cleanup() {
    if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; fi
    rm -rf -- "$temporary"
}
trap cleanup EXIT
app="$temporary/DownloaderSmoke.app"
mkdir -p "$app"
python3 - "$app" <<'PY'
import plistlib, sys
from pathlib import Path
app = Path(sys.argv[1])
(app/'Info.plist').write_bytes(plistlib.dumps({
    'CFBundleExecutable':'smoke', 'CFBundleIdentifier':'com.jeetraj.tdrive.downloader-smoke',
    'CFBundleName':'DownloaderSmoke', 'CFBundlePackageType':'APPL',
    'CFBundleVersion':'1', 'CFBundleShortVersionString':'1.0', 'MinimumOSVersion':'16.0',
}))
PY
python3 build/ios/mediadl/package_runtime.py \
    --source build/ios/mediadl/cache/py/Python.xcframework \
    --wheel build/ios/mediadl/cache/yt_dlp.whl \
    --certifi build/ios/mediadl/cache/certifi.whl \
    --app "$app" --platform simulator --sign -
xcrun --sdk iphonesimulator clang -fobjc-arc -fblocks \
    -target arm64-apple-ios16.0-simulator \
    -isysroot "$(xcrun --sdk iphonesimulator --show-sdk-path)" \
    -Fbuild/ios/mediadl/cache/py/Python.xcframework/ios-arm64_x86_64-simulator \
    -framework Python -Wl,-rpath,@executable_path/Frameworks \
    -framework Foundation -framework UIKit -framework WebKit \
    build/ios/mediadl/smoke.m -o "$app/smoke"
codesign --force --sign - --timestamp=none "$app"
device="$(xcrun simctl list devices available -j | python3 -c 'import json,sys; data=json.load(sys.stdin); print(next(d["udid"] for devices in data["devices"].values() for d in devices if "iPhone" in d["name"]))')"
xcrun simctl boot "$device" 2>/dev/null || true
xcrun simctl bootstatus "$device" -b
xcrun simctl install "$device" "$app"
installed="$(xcrun simctl get_app_container "$device" com.jeetraj.tdrive.downloader-smoke app)"
python3 - "$workspace" "$temporary/port" <<'PY' &
import http.server, os, socketserver, sys
from pathlib import Path
os.chdir(sys.argv[1])
class LocalServer(http.server.ThreadingHTTPServer):
    def server_bind(self):
        # Avoid a reverse-DNS lookup on hosted macOS runners.
        socketserver.TCPServer.server_bind(self)
        self.server_name = 'localhost'
        self.server_port = self.server_address[1]
server=LocalServer(('127.0.0.1',0),http.server.SimpleHTTPRequestHandler)
Path(sys.argv[2]).write_text(str(server.server_port))
server.serve_forever()
PY
server_pid=$!
for attempt in $(seq 1 600); do
    [ -f "$temporary/port" ] && break
    kill -0 "$server_pid" 2>/dev/null || { echo 'Fixture server exited before startup' >&2; exit 1; }
    sleep 0.1
done
test -f "$temporary/port" || { echo 'Fixture server did not start within 60 seconds' >&2; exit 1; }
port="$(cat "$temporary/port")"
fixture=frontend/e2e/fixtures/tiny.mp4
test -f "$fixture"
digest="$(shasum -a 256 "$fixture" | awk '{print $1}')"
xcrun simctl spawn "$device" "$installed/smoke" "http://127.0.0.1:$port/$fixture" "$digest"
