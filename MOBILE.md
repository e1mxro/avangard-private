# AVANGARD Mobile clients

Native iOS / Android clients for the AVANGARD protocol. The Go core (`pkg/...`) is
shared between platforms via [`gomobile`](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile)
and exposed by the small wrapper package [`mobile/avmobile`](mobile/avmobile/avmobile.go).

## Status matrix

| Platform | Phase | Branch | Output |
|---|---|---|---|
| Android | **v0.1 (current)** — single-screen, SOCKS5 only | `mobile/android` | `avangard-android-debug.apk` (CI artifact) |
| Android | v0.2 (next) — VpnService + tun2socks | TBD | system-wide tunnel |
| Android | v0.3 — profiles, subscription, per-app routing, themes | TBD | "Hiddify-grade" UI |
| iOS | v0.1 (planned) — Swift + NEPacketTunnelProvider | TBD | `.ipa` from macOS Actions runner |

## Android v0.1 — install on your phone

1. Open https://github.com/e1mxro/avangard-private/actions/workflows/android.yml
2. Pick the latest **green** run on branch `mobile/android` (or `init` after merge).
3. Download artifact `avangard-android-debug.zip`. Extract → `app-debug.apk`.
4. Transfer the APK to your phone (USB / Telegram / cloud / `adb install`) and open it.
5. Allow "install from unknown sources" if Android prompts.
6. Open the app, paste your `avangard://...` URI, pick TCP or QUIC, press **Connect**.
7. The app pins a notification: `SOCKS5 ready on 127.0.0.1:18964`.
8. Configure apps to use that SOCKS5 — examples below.

### Configure apps to use the SOCKS5 proxy

| App | Where |
|---|---|
| **Firefox for Android** | Settings → Add-ons → Install [SOCKS5 proxy](https://addons.mozilla.org/en-US/firefox/addon/socks5-configurator/) → set `127.0.0.1:18964` |
| **Bromite** / **Mulch** | Launch with intent extras, or use `pac://` URL pointing at SOCKS5 |
| **Telegram** | Settings → Data and Storage → Proxy → Add Proxy → SOCKS5, host `127.0.0.1`, port `18964` |
| **Discord/WhatsApp/native apps** | Won't work without VpnService — wait for v0.2 or use [ProxyDroid](https://f-droid.org/en/packages/org.proxydroid/) (root) / [SocksDroid](https://f-droid.org/en/packages/net.typeblog.socks/) (no root) to redirect specific apps |

For system-wide tunneling with no per-app config, install **SocksDroid** alongside our app:
1. F-Droid → SocksDroid.
2. Server: `127.0.0.1`, Port: `18964`.
3. ✓ "Auto Connect" → tap **CONNECT**.
4. SocksDroid creates its own VpnService that forwards every TCP packet to our SOCKS5.

This is a stop-gap until our v0.2 ships with built-in VpnService.

## iOS — install paths

Until our native iOS app is ready, the practical options are:

### Option A: Apple Developer ($99/yr) → permanent install
- Sign up: https://developer.apple.com/programs/enroll/
- Build the app via Xcode (or our macOS GitHub Actions workflow, see below).
- Upload to TestFlight or distribute as `.ipa`.

### Option B: Sideloadly + Free Apple ID (7-day refresh, free)
- Install Sideloadly: https://sideloadly.io
- Sign in with your Apple ID. Connect iPhone by USB.
- Drag the `.ipa` into Sideloadly → install.
- Repeat every 7 days when the signature expires.

### Option C: AltStore + Free Apple ID (7-day auto-refresh, free)
- Install AltServer on Mac/Windows: https://altstore.io
- Install AltStore on iPhone via AltServer.
- Add `.ipa` from "Files" → tap to install.
- AltServer auto-refreshes signatures while it's running on the same Wi-Fi.

### Option D: TrollStore (permanent, FREE, requires iOS ≤ 17.0)
- iPhone XS–15, iOS 14.0–17.0 (NOT 17.1+) — install TrollStore via https://ios.cfw.guide/installing-trollstore
- Once installed, any `.ipa` installs **permanently** with no refresh.

### Option E: Streisand on App Store ($1.99) + intermediate Linux box (works today)
- Run `avangard-client socks5 --listen 0.0.0.0:18964` on a Linux box / second VPS / Termux on an old Android.
- Buy Streisand from the App Store: https://apps.apple.com/app/streisand/id6450534064
- Add a SOCKS5 server pointing at that intermediate box.
- All iPhone traffic now goes through AVANGARD.

## Building

### Android
See [mobile/android/README.md](mobile/android/README.md). CI: `.github/workflows/android.yml`.

### iOS
Planned. Requires Xcode 15+ on macOS. Will produce `.xcframework` from the same `mobile/avmobile` package via:
```
gomobile bind -target=ios,iossimulator -o AvangardMobile.xcframework ./mobile/avmobile
```
The Swift app embeds the framework and a `NEPacketTunnelProvider` extension calls `Avmobile.Start()`.

CI workflow will run on `macos-14` GitHub-hosted runners. Public repos get free macOS minutes; private repos
incur ~$0.08/minute (≈ $5–15/month for an active dev cycle). Provide your Apple Developer cert + provisioning
profile as repo secrets:
- `APPLE_DEVELOPER_TEAM_ID`
- `APPLE_DEVELOPER_CERT_P12_BASE64`
- `APPLE_DEVELOPER_CERT_PASSWORD`
- `APPLE_PROVISIONING_PROFILE_BASE64`
