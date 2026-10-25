#!/bin/sh

set -eu

APP_ROOT="${PROJECT_DIR}/../../.."
PLATFORM="${PLATFORM_NAME:-iphonesimulator}"
SDK_PATH="${SDKROOT:-$(xcrun --sdk "${PLATFORM}" --show-sdk-path)}"
MIN_IOS_VERSION="${IPHONEOS_DEPLOYMENT_TARGET:-16.0}"

if [ "${PLATFORM}" = "iphonesimulator" ]; then
    GO_TARGET="arm64-apple-ios${MIN_IOS_VERSION}-simulator"
    MINIMUM_FLAG="-mios-simulator-version-min=${MIN_IOS_VERSION}"
else
    GO_TARGET="arm64-apple-ios${MIN_IOS_VERSION}"
    MINIMUM_FLAG="-miphoneos-version-min=${MIN_IOS_VERSION}"
fi

export GOOS=ios
export GOARCH=arm64
export CGO_ENABLED=1
export CGO_CFLAGS="-isysroot ${SDK_PATH} -target ${GO_TARGET} ${MINIMUM_FLAG}"
export CGO_LDFLAGS="-isysroot ${SDK_PATH} -target ${GO_TARGET}"

cd "${APP_ROOT}"
mkdir -p "${APP_ROOT}/bin"

if [ ! -f build/ios/xcode/overlay.json ]; then
    wails3 ios overlay:gen -out build/ios/xcode/overlay.json -config build/config.yml
fi

if [ "${CONFIGURATION:-Debug}" = "Release" ]; then
    wails3 task common:build:frontend BUILD_FLAGS="-tags production,ios"
    go build -tags production,ios -trimpath -buildvcs=false -ldflags="-w -s -X main.appVersion=${MARKETING_VERSION}" \
        -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o "bin/${PRODUCT_NAME}.a"
else
    go build -tags ios,debug -buildvcs=false -gcflags=all="-l" \
        -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o "bin/${PRODUCT_NAME}.a"
fi

# Xcode links the same runtime as the Taskfile packaging path.
if [ "${PLATFORM}" = "iphonesimulator" ]; then
    PYTHON_PLATFORM=simulator
else
    PYTHON_PLATFORM=device
fi
python3 build/ios/mediadl/package_runtime.py \
    --source build/ios/mediadl/cache/py/Python.xcframework \
    --wheel build/ios/mediadl/cache/yt_dlp.whl \
    --certifi build/ios/mediadl/cache/certifi.whl \
    --app "${TARGET_BUILD_DIR}/${WRAPPER_NAME}" --platform "${PYTHON_PLATFORM}" \
    --sign "${EXPANDED_CODE_SIGN_IDENTITY:--}"
