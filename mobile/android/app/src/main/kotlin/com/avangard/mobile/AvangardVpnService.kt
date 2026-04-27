package com.avangard.mobile

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
// gomobile bind without -javapkg generates a top-level package named after the
// Go package; for `package avmobile` the resulting Java class is `avmobile.Avmobile`.
import avmobile.Avmobile

/**
 * AvangardVpnService is the Android VpnService implementation that turns the
 * AVANGARD tunnel into a system-wide VPN. Every IP packet from every app on
 * the device flows through:
 *
 *     OS netstack → tun (this VPN interface) → tun2socks (in Go via avmobile)
 *                 → 127.0.0.1:18964 SOCKS5  → AVANGARD tunnel → server
 *
 * The legacy [AvangardForegroundService] is kept for SOCKS5-only mode where
 * the user wants per-app proxy configuration instead of capturing all device
 * traffic.
 */
class AvangardVpnService : VpnService() {

    private var tunInterface: ParcelFileDescriptor? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START
        when (action) {
            ACTION_START -> {
                val uri = intent?.getStringExtra(EXTRA_URI).orEmpty()
                val transport = intent?.getStringExtra(EXTRA_TRANSPORT) ?: "tcp"
                val perAppMode = intent?.getStringExtra(EXTRA_PER_APP_MODE) ?: PER_APP_MODE_ALL
                val perAppPackages = intent?.getStringArrayExtra(EXTRA_PER_APP_PACKAGES)?.toList().orEmpty()
                start(uri, transport, perAppMode, perAppPackages)
            }
            ACTION_STOP -> stop()
        }
        return START_STICKY
    }

    private fun start(
        uri: String,
        transport: String,
        perAppMode: String,
        perAppPackages: List<String>,
    ) {
        // Guard against duplicate START intents (rapid double-tap before the
        // UI's 1s poll loop sees running=true; START_STICKY redelivery; etc.).
        // Without this, Avmobile.start would throw "already running" and the
        // catch block below would happily tear down the working tunnel.
        if (tunInterface != null) {
            Log.i(TAG, "start ignored: tunnel already active")
            return
        }
        ensureChannel()
        startForeground(NOTIFICATION_ID, buildNotification(getString(R.string.notif_connecting)))
        try {
            // 1. Start AVANGARD SOCKS5 on loopback. tun2socks will forward
            //    every captured TCP/UDP flow to it.
            Avmobile.start(uri, LISTEN_ADDR, transport)

            // 2. Establish the VPN interface with a default route. The
            //    private 10.42.0.x range is conventional for VPN apps.
            val builder = Builder()
                .setSession(getString(R.string.app_name))
                .setMtu(MTU)
                .addAddress(VPN_ADDRESS, 32)
                .addRoute("0.0.0.0", 0)
                .addRoute("::", 0)
                .addDnsServer(DNS_PRIMARY)
                .addDnsServer(DNS_SECONDARY)

            // Exclude our own app from the VPN so the upstream AVANGARD dial
            // doesn't loop back through the tun we just installed.
            //
            // VpnService.Builder forbids mixing addAllowedApplication with
            // addDisallowedApplication on the same builder — the second call
            // throws UnsupportedOperationException. So we only self-exclude
            // when we're going to use the disallow list (modes ALL and
            // DISALLOW). In ALLOW mode the self-package is implicitly
            // excluded because it's not in the user's allow list, and
            // applyPerAppRouting() also skips it defensively.
            if (perAppMode != PER_APP_MODE_ALLOW) {
                try {
                    builder.addDisallowedApplication(packageName)
                } catch (t: Throwable) {
                    Log.w(TAG, "addDisallowedApplication failed", t)
                }
            }

            applyPerAppRouting(builder, perAppMode, perAppPackages)

            val pfd = builder.establish()
                ?: throw IllegalStateException("VpnService.Builder.establish() returned null")
            tunInterface = pfd

            // 3. Hand the fd to the gomobile-bound tun2socks engine.
            //    gomobile maps Go `int` to Kotlin `Long`.
            Avmobile.startTun(pfd.fd.toLong(), MTU.toLong(), LISTEN_ADDR)

            updateNotification(getString(R.string.notif_vpn_active))
            Log.i(TAG, "VPN up: fd=${pfd.fd} mtu=$MTU socks=$LISTEN_ADDR")
        } catch (t: Throwable) {
            Log.e(TAG, "start failed", t)
            updateNotification(getString(R.string.notif_error, t.message ?: "unknown"))
            // Best-effort cleanup of the partial state.
            try { Avmobile.stopTun() } catch (_: Throwable) {}
            try { Avmobile.stop() } catch (_: Throwable) {}
            try { tunInterface?.close() } catch (_: Throwable) {}
            tunInterface = null
            stopSelf()
        }
    }

    private fun stop() {
        try { Avmobile.stopTun() } catch (t: Throwable) { Log.w(TAG, "stopTun", t) }
        try { Avmobile.stop() } catch (t: Throwable) { Log.w(TAG, "stop", t) }
        try { tunInterface?.close() } catch (_: Throwable) {}
        tunInterface = null
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() {
        try { Avmobile.stopTun() } catch (_: Throwable) {}
        try { Avmobile.stop() } catch (_: Throwable) {}
        try { tunInterface?.close() } catch (_: Throwable) {}
        tunInterface = null
        super.onDestroy()
    }

    override fun onRevoke() {
        // Triggered when the user disables the VPN via system settings or a
        // different VPN app takes over.
        stop()
        super.onRevoke()
    }

    private fun applyPerAppRouting(
        builder: Builder,
        mode: String,
        packages: List<String>,
    ) {
        if (mode == PER_APP_MODE_ALL || packages.isEmpty()) return
        val ours = packageName
        for (pkg in packages) {
            if (pkg == ours) continue
            try {
                when (mode) {
                    PER_APP_MODE_ALLOW -> builder.addAllowedApplication(pkg)
                    PER_APP_MODE_DISALLOW -> builder.addDisallowedApplication(pkg)
                }
            } catch (t: Throwable) {
                Log.w(TAG, "per-app routing skipped for $pkg ($mode)", t)
            }
        }
    }

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val nm = getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) != null) return
        val ch = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.channel_vpn_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = getString(R.string.channel_vpn_desc)
            setShowBadge(false)
        }
        nm.createNotificationChannel(ch)
    }

    private fun buildNotification(text: String) = NotificationCompat.Builder(this, CHANNEL_ID)
        .setSmallIcon(android.R.drawable.ic_lock_lock)
        .setContentTitle(getString(R.string.app_name))
        .setContentText(text)
        .setContentIntent(
            PendingIntent.getActivity(
                this, 0, Intent(this, MainActivity::class.java),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            ),
        )
        .addAction(
            0, getString(R.string.notif_action_stop),
            PendingIntent.getService(
                this, 1,
                Intent(this, AvangardVpnService::class.java).apply { action = ACTION_STOP },
                PendingIntent.FLAG_IMMUTABLE,
            ),
        )
        .setOngoing(true)
        .setOnlyAlertOnce(true)
        .build()

    private fun updateNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, buildNotification(text))
    }

    companion object {
        private const val TAG = "AvangardVpn"
        const val CHANNEL_ID = "avangard-vpn"
        const val NOTIFICATION_ID = 1002
        const val LISTEN_ADDR = "127.0.0.1:18964"
        const val VPN_ADDRESS = "10.42.0.1"
        const val DNS_PRIMARY = "1.1.1.1"
        const val DNS_SECONDARY = "8.8.8.8"
        const val MTU = 1500

        const val ACTION_START = "com.avangard.mobile.vpn.START"
        const val ACTION_STOP = "com.avangard.mobile.vpn.STOP"
        const val EXTRA_URI = "uri"
        const val EXTRA_TRANSPORT = "transport"
        const val EXTRA_PER_APP_MODE = "per_app_mode"
        const val EXTRA_PER_APP_PACKAGES = "per_app_packages"

        const val PER_APP_MODE_ALL = "ALL"
        const val PER_APP_MODE_ALLOW = "ALLOW"
        const val PER_APP_MODE_DISALLOW = "DISALLOW"

        /** Build the start Intent — caller must call [VpnService.prepare] first. */
        fun startIntent(
            ctx: Context,
            uri: String,
            transport: String,
            perAppMode: String = PER_APP_MODE_ALL,
            perAppPackages: List<String> = emptyList(),
        ): Intent = Intent(ctx, AvangardVpnService::class.java).apply {
            action = ACTION_START
            putExtra(EXTRA_URI, uri)
            putExtra(EXTRA_TRANSPORT, transport)
            putExtra(EXTRA_PER_APP_MODE, perAppMode)
            putExtra(EXTRA_PER_APP_PACKAGES, perAppPackages.toTypedArray())
        }

        fun stopIntent(ctx: Context): Intent =
            Intent(ctx, AvangardVpnService::class.java).apply {
                action = ACTION_STOP
            }
    }
}
