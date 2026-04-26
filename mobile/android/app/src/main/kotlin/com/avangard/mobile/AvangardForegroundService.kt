package com.avangard.mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import avmobile.Avmobile

/**
 * AvangardForegroundService keeps the AVANGARD client + SOCKS5 listener alive
 * while the app is in the background. It does NOT (yet) capture system traffic
 * via VpnService — see roadmap for v0.2.
 *
 * Apps that want to use the tunnel must be configured to send traffic through
 * the SOCKS5 proxy on 127.0.0.1:18964 (e.g. Firefox, ProxyDroid, Termux).
 */
class AvangardForegroundService : Service() {

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START
        when (action) {
            ACTION_START -> {
                val uri = intent?.getStringExtra(EXTRA_URI).orEmpty()
                val transport = intent?.getStringExtra(EXTRA_TRANSPORT) ?: "tcp"
                start(uri, transport)
            }
            ACTION_STOP -> stop()
        }
        return START_STICKY
    }

    private fun start(uri: String, transport: String) {
        ensureChannel()
        val notification = buildNotification(getString(R.string.notif_connecting))
        startForeground(NOTIFICATION_ID, notification)

        try {
            Avmobile.start(uri, LISTEN_ADDR, transport)
            updateNotification(getString(R.string.notif_connected, LISTEN_ADDR))
        } catch (t: Throwable) {
            Log.e(TAG, "start failed", t)
            updateNotification(getString(R.string.notif_error, t.message ?: "unknown"))
        }
    }

    private fun stop() {
        try {
            Avmobile.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "stop", t)
        }
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() {
        try {
            Avmobile.stop()
        } catch (_: Throwable) {
        }
        super.onDestroy()
    }

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val nm = getSystemService(NotificationManager::class.java)
        val existing = nm.getNotificationChannel(CHANNEL_ID)
        if (existing != null) return
        val ch = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.channel_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = getString(R.string.channel_desc)
            setShowBadge(false)
        }
        nm.createNotificationChannel(ch)
    }

    private fun buildNotification(text: String): Notification {
        val openIntent = Intent(this, MainActivity::class.java)
        val openPi = PendingIntent.getActivity(
            this, 0, openIntent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val stopIntent = Intent(this, AvangardForegroundService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPi = PendingIntent.getService(
            this, 1, stopIntent, PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(openPi)
            .addAction(0, getString(R.string.notif_action_stop), stopPi)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .build()
    }

    private fun updateNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIFICATION_ID, buildNotification(text))
    }

    companion object {
        private const val TAG = "AvangardFgs"
        const val CHANNEL_ID = "avangard-tunnel"
        const val NOTIFICATION_ID = 1001
        const val LISTEN_ADDR = "127.0.0.1:18964"

        const val ACTION_START = "com.avangard.mobile.action.START"
        const val ACTION_STOP = "com.avangard.mobile.action.STOP"
        const val EXTRA_URI = "uri"
        const val EXTRA_TRANSPORT = "transport"

        fun start(ctx: Context, uri: String, transport: String) {
            val i = Intent(ctx, AvangardForegroundService::class.java).apply {
                action = ACTION_START
                putExtra(EXTRA_URI, uri)
                putExtra(EXTRA_TRANSPORT, transport)
            }
            ctx.startForegroundService(i)
        }

        fun stop(ctx: Context) {
            val i = Intent(ctx, AvangardForegroundService::class.java).apply {
                action = ACTION_STOP
            }
            ctx.startService(i)
        }
    }
}
