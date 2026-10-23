#!/usr/bin/env bash
#
# Packages a release APK and copies it to build/bin/.
#
#   package.sh               # both ABIs -> build/bin/MrRSS.apk
#   package.sh --abi arm64   # phone only  -> build/bin/MrRSS-arm64-v8a.apk
#   package.sh --abi x86_64  # emulator only -> build/bin/MrRSS-x86_64.apk
#
# `dev.sh` deliberately keeps only the emulator's library to stay fast, so a
# release package rebuilds the native library itself. Mirrors what
# `task android:package:fat` does, for an environment without the Task CLI.
#
set -euo pipefail

ABI="all"
while [ $# -gt 0 ]; do
  case "$1" in
    --abi)
      ABI="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown option: $1 (usage: $0 [--abi all|arm64|x86_64])" >&2
      exit 2
      ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if [ -z "${ANDROID_HOME:-}" ] && [ -n "${LOCALAPPDATA:-}" ]; then
  # %LOCALAPPDATA% holds backslashes; bash needs forward slashes for -d checks.
  ANDROID_HOME="${LOCALAPPDATA//\\//}/Android/Sdk"
fi
if [ -z "${ANDROID_HOME:-}" ] || [ ! -d "$ANDROID_HOME" ]; then
  echo "error: Android SDK not found (set ANDROID_HOME to the SDK root)" >&2
  exit 1
fi
NDK_ROOT="${ANDROID_NDK_HOME:-$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)}"
if [ -z "$NDK_ROOT" ] || [ ! -d "$NDK_ROOT" ]; then
  echo "error: no NDK installed under $ANDROID_HOME/ndk" >&2
  exit 1
fi

HOST_TAG="linux-x86_64"
GRADLE="./gradlew"
CC_SUFFIX=""
case "$(uname -s)" in
  Darwin) HOST_TAG="darwin-x86_64" ;;
  MINGW*|MSYS*|CYGWIN*)
    HOST_TAG="windows-x86_64"
    GRADLE="./gradlew.bat"
    CC_SUFFIX=".cmd"
    ;;
esac
TOOLCHAIN="$NDK_ROOT/toolchains/llvm/prebuilt/$HOST_TAG/bin"

build_lib() {
  local arch="$1" cc="$2" out="$3"
  mkdir -p "$(dirname "$out")"
  GOOS=android GOARCH="$arch" CGO_ENABLED=1 CC="$TOOLCHAIN/$cc$CC_SUFFIX" \
    go build -tags production,android -trimpath -buildvcs=false \
    -ldflags="-w -s" -buildmode=c-shared -o "$out" .
}

build_arm64() {
  echo "--- arm64"
  build_lib arm64 aarch64-linux-android21-clang \
    build/android/app/src/main/jniLibs/arm64-v8a/libwails.so
}

build_x86_64() {
  echo "--- x86_64"
  build_lib amd64 x86_64-linux-android21-clang \
    build/android/app/src/main/jniLibs/x86_64/libwails.so
}

GRADLE_ARGS=()
case "$ABI" in
  all)
    build_arm64
    build_x86_64
    OUT="build/bin/MrRSS.apk"
    ;;
  arm64)
    build_arm64
    GRADLE_ARGS=(-PabiFilters=arm64-v8a)
    OUT="build/bin/MrRSS-arm64-v8a.apk"
    ;;
  x86_64)
    build_x86_64
    GRADLE_ARGS=(-PabiFilters=x86_64)
    OUT="build/bin/MrRSS-x86_64.apk"
    ;;
  *)
    echo "error: unknown ABI '$ABI' (use all, arm64 or x86_64)" >&2
    exit 2
    ;;
esac

# Android Studio's bundled JDK is the usual answer on a Windows host that has
# not exported JAVA_HOME; CI and Linux/macOS setups pass their own.
GRADLE_ENV=()
if [ -z "${JAVA_HOME:-}" ] && [ -d "/c/Program Files/Android/Android Studio/jbr" ]; then
  GRADLE_ENV=(JAVA_HOME="/c/Program Files/Android/Android Studio/jbr")
fi

echo "--- assembleRelease"
(
  cd build/android
  env "${GRADLE_ENV[@]}" ANDROID_HOME="$ANDROID_HOME" "$GRADLE" assembleRelease -q "${GRADLE_ARGS[@]}"
)

APK="build/android/app/build/outputs/apk/release/app-release.apk"
mkdir -p build/bin
cp "$APK" "$OUT"
echo "==> $OUT ($(du -h "$OUT" | cut -f1))"
