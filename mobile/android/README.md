# AVANGARD Android — v0.1.0-alpha

Minimum-viable Android client for AVANGARD.

## Status: v0.1 (current)

- Foreground Service running the AVANGARD tunnel + a local SOCKS5 listener on `127.0.0.1:18964`.
- Single-screen Compose UI: paste an `avangard://` URI, choose TCP or QUIC, press **Connect**.
- App-level proxying only: Firefox, Bromite, Telegram, Termux, etc. need to be configured to use SOCKS5 `127.0.0.1:18964`.

## Roadmap

- **v0.2** — VpnService + tun2socks integration so the OS routes everything through the tunnel without per-app config.
- **v0.3** — multiple profiles, subscription import, QR scanner, per-app routing rules, traffic stats, light/dark/AMOLED themes.

## Building locally

```bash
# 1. Build the gomobile AAR (requires Go 1.25+ and Android NDK r26+)
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/26.1.10909125
./mobile/scripts/build-aar.sh

# 2. Build the APK
cd mobile/android
gradle wrapper --gradle-version 8.9 --distribution-type bin
./gradlew assembleDebug
# → app/build/outputs/apk/debug/app-debug.apk
```

## CI

GitHub Actions workflow: [`.github/workflows/android.yml`](../../.github/workflows/android.yml).

Each push to `mobile/**` produces a downloadable debug APK as a workflow artifact. Pick the latest run from
`https://github.com/e1mxro/avangard-private/actions/workflows/android.yml`, download `avangard-android-debug.zip`,
extract, and `adb install app-debug.apk` (or transfer the `.apk` to your phone and tap to install — make sure
"Install from unknown sources" is allowed for your file manager).
