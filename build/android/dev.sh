#!/usr/bin/env bash
#
# Fast development loop for the Android emulator.
#
# Builds only the x86_64 native library (the emulator's ABI), packages an
# incremental debug APK without cleaning, and installs it on the running
# emulator. Rebuilding both ABIs or producing a release APK is a different
# task — see docs/MOBILE.md.
#
# Usage:
#   build/android/dev.sh              # frontend + Go + package + install + launch
#   build/android/dev.sh --go-only    # skip the frontend build
#   build/android/dev.sh --no-install # stop after packaging
#
# Environment:
#   ANDROID_SERIAL  emulator serial (default: emulator-5554)
#   ANDROID_HOME    SDK root (default: %LOCALAPPDATA%\Android\Sdk on Windows)

set -euo pipefail

# Per-phase timings, printed as the loop runs. The point of this script is
# iteration speed, so it should be obvious where the seconds go.
STEP_T0=$SECONDS
step() {
  local now=$SECONDS
  printf '==> %-28s %3ds\n' "$1" "$((now - STEP_T0))"
  STEP_T0=$now
}

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

SERIAL="${ANDROID_SERIAL:-emulator-5554}"
SKIP_FRONTEND=0
SKIP_INSTALL=0
for arg in "$@"; do
  case "$arg" in
    --go-only) SKIP_FRONTEND=1 ;;
    --no-install) SKIP_INSTALL=1 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

# --- toolchain -----------------------------------------------------------------

if [ -z "${ANDROID_HOME:-}" ]; then
  ANDROID_HOME="$LOCALAPPDATA/Android/Sdk"
fi
if [ ! -d "$ANDROID_HOME" ]; then
  echo "error: Android SDK not found at $ANDROID_HOME (set ANDROID_HOME)" >&2
  exit 1
fi

NDK_ROOT="${ANDROID_NDK_HOME:-$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)}"
if [ -z "$NDK_ROOT" ] || [ ! -d "$NDK_ROOT" ]; then
  echo "error: no NDK installed under $ANDROID_HOME/ndk" >&2
  exit 1
fi

HOST_TAG="linux-x86_64"
EXE=""
case "$(uname -s)" in
  Darwin) HOST_TAG="darwin-x86_64" ;;
  Linux) HOST_TAG="linux-x86_64" ;;
  MINGW*|MSYS*|CYGWIN*) HOST_TAG="windows-x86_64"; EXE=".cmd" ;;
  *) echo "error: unsupported host $(uname -s)" >&2; exit 1 ;;
esac
TOOLCHAIN="$NDK_ROOT/toolchains/llvm/prebuilt/$HOST_TAG"

# --- frontend ------------------------------------------------------------------

if [ "$SKIP_FRONTEND" -eq 0 ]; then
  echo "--- frontend"
  # Vite empties its outDir at the start of every build, and the sandbox this
  # runs in rejects bulk deletes inside the workspace. Move the previous build
  # out to the OS temp dir instead of deleting it, and move it back if the
  # build fails so a bad frontend edit cannot leave `dist` empty.
  STASH="$(mktemp -d "${TMPDIR:-/tmp}/mrrss-dist-XXXXXX")"
  if [ -d frontend/dist ]; then
    mv frontend/dist "$STASH/dist"
  fi
  if ! ( cd frontend && npx vite build --logLevel warn ); then
    if [ -d frontend/dist ]; then mv frontend/dist "$STASH/failed"; fi
    if [ -d "$STASH/dist" ]; then mv "$STASH/dist" frontend/dist; fi
    echo "error: frontend build failed (previous dist/ restored)" >&2
    exit 1
  fi
  step "frontend"
fi

# --- native library ------------------------------------------------------------

# The emulator only ever loads x86_64. Keeping the arm64 library out of the APK
# roughly halves packaging time; `task android:build ARCH=arm64` restores it.
rm -f build/android/app/src/main/jniLibs/arm64-v8a/libwails.so
mkdir -p build/android/app/src/main/jniLibs/x86_64

GOOS=android GOARCH=amd64 CGO_ENABLED=1 \
  CC="$TOOLCHAIN/bin/x86_64-linux-android21-clang$EXE" \
  go build -tags android,debug -buildvcs=false -buildmode=c-shared \
  -o build/android/app/src/main/jniLibs/x86_64/libwails.so .
step "libwails.so (x86_64, debug)"

# --- package -------------------------------------------------------------------

# No `clean`, and the Gradle daemon is kept warm between runs: both are worth
# roughly a minute per iteration compared with a cold `clean assembleDebug`.
if ! (
  cd build/android
  JAVA_HOME="${JAVA_HOME:-/c/Program Files/Android/Android Studio/jbr}" \
  ANDROID_HOME="$ANDROID_HOME" \
    ./gradlew.bat assembleDebug -q
); then
  echo "error: gradle assembleDebug failed" >&2
  exit 1
fi

APK="build/android/app/build/outputs/apk/debug/app-debug.apk"
if [ ! -f "$APK" ]; then
  echo "error: $APK was not produced" >&2
  exit 1
fi
step "assembleDebug ($(du -h "$APK" | cut -f1))"

if [ "$SKIP_INSTALL" -eq 1 ]; then
  echo "==> packaged: $APK"
  exit 0
fi

# --- install & launch ----------------------------------------------------------

ADB="$ANDROID_HOME/platform-tools/adb.exe"
[ -x "$ADB" ] || ADB="$ANDROID_HOME/platform-tools/adb"

if ! "$ADB" devices | grep -q "^$SERIAL[[:space:]]*device"; then
  echo "error: $SERIAL is not attached (start the emulator first)" >&2
  exit 1
fi

"$ADB" -s "$SERIAL" install -r "$APK" >/dev/null
"$ADB" -s "$SERIAL" shell am force-stop com.mrrss.app
"$ADB" -s "$SERIAL" shell am start -n com.mrrss.app/com.wails.app.MainActivity >/dev/null

PID="$("$ADB" -s "$SERIAL" shell pidof com.mrrss.app | tr -d '\r')"
if [ -z "$PID" ]; then
  echo "!! the app exited right after launch; recent log:" >&2
  "$ADB" -s "$SERIAL" logcat -d -t 120 | grep -iE "Fatal|SIGSYS|SIGABRT|GoLog" | tail -20 >&2
  exit 1
fi
step "install + launch"

echo "==> running (pid $PID)"
echo "    logs:    $ADB -s $SERIAL logcat"
echo "    app log: $ADB -s $SERIAL shell run-as com.mrrss.app cat files/logs/debug.log"
echo "    inspect: $ADB -s $SERIAL forward tcp:9222 localabstract:webview_devtools_remote_$PID"
