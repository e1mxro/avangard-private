# AVANGARD Mobile clients

Native iOS / Android clients for the AVANGARD protocol. The Go core (`pkg/...`) is
shared between platforms via [`gomobile`](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile)
and exposed by the small wrapper package [`mobile/avmobile`](mobile/avmobile/avmobile.go).

## Status matrix

| Platform | Phase | Branch | Output |
|---|---|---|---|
| Android | v0.1 — SOCKS5 only | `mobile/android` | `avangard-android-debug.apk` (CI artifact) |
| Android | v0.2 — VpnService + tun2socks (system-wide) | `mobile/android` | system-wide tunnel |
| Android | **v0.3 (current)** — profiles, subscription, QR, per-app routing, themes | `mobile/android` | "Hiddify-grade" UI |
| Android | v0.4 (planned) — stats, latency, always-on, signed release | TBD | release-signed APK |
| iOS | v0.1 (planned) — Swift + NEPacketTunnelProvider | TBD | `.ipa` from macOS Actions runner |

## Android v0.3 — features

- **Multi-profile storage** — keep any number of `avangard://...` configurations; tap one to make it active. Profiles live in DataStore (`/data/data/com.avangard.mobile/...`).
- **Subscription import** — paste a URL that returns a list of `avangard://` URIs (newline-separated, optionally base64-wrapped à la v2rayN). Bulk-imports new profiles, deduplicates by URI.
- **QR scanner** — from the profile editor, tap the QR icon to scan an `avangard://` URI directly with the camera (ZXing-backed).
- **Per-app routing** — Settings → Per-app routing. Three modes:
    - *All apps* (default) — every app uses AVANGARD.
    - *Only selected* — only the checked apps go through AVANGARD; everything else uses the direct connection (great for split-tunnel work setups).
    - *Bypass selected* — every app goes through AVANGARD except the checked ones (e.g. banking apps that geo-fence).
- **Themes** — System / Light / Dark / AMOLED (pure-black for OLED battery saving). Live-applied without restart.
- **System VPN / SOCKS5** toggle — from the v0.2 baseline, kept for power users who want a local SOCKS5 instead of the full tun.

## Android — install on your phone

1. Open https://github.com/e1mxro/avangard-private/actions/workflows/android.yml
2. Pick the latest **green** run on branch `mobile/android` (or `init` after merge).
3. Download artifact `avangard-android-debug.zip`. Extract → `app-debug.apk`.
4. Transfer the APK to your phone (USB / Telegram / cloud / `adb install`) and open it.
5. Allow "install from unknown sources" if Android prompts.
6. Open the app → **Profiles** tab → **Add profile** (or scan a QR via the camera button) → paste your `avangard://...` URI → Save.
7. Switch to **Home** → press **Connect** → accept the system VPN consent dialog.
8. The status bar key icon appears and the notification reads `System-wide VPN active`. Every app on the device now routes through AVANGARD.

For SOCKS5-only mode (no VPN consent, per-app proxy configuration), flip the **Mode** switch on the Home screen to *SOCKS5 only* before connecting; the listener stays at `127.0.0.1:18964`.

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
