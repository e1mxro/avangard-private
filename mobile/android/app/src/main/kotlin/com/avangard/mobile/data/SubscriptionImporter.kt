package com.avangard.mobile.data

import android.util.Base64
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

/**
 * Downloads a subscription URL and parses its body into a list of [Profile]s.
 *
 * Supported body formats:
 *   1. Newline-separated `avangard://...` URIs (one per line).
 *   2. Base64-encoded form of (1) — common with v2rayN-style subscriptions.
 *   3. Comments (`#` prefix) and blank lines are ignored.
 *
 * The host returns plain text by convention; we don't speak SIP008 / Clash
 * yet — that would belong in v0.4.
 */
object SubscriptionImporter {

    private val client by lazy {
        OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(15, TimeUnit.SECONDS)
            .build()
    }

    /**
     * Fetches [url] and extracts AVANGARD URIs. Each URI becomes a [Profile]
     * with [Profile.subscriptionUrl] set to [url] for tracking.
     *
     * Names are auto-generated from the URI's host:port.
     */
    fun fetch(url: String): Result<List<Profile>> = runCatching {
        require(url.startsWith("http://") || url.startsWith("https://")) {
            "subscription URL must be http(s)"
        }
        val body = client.newCall(Request.Builder().url(url).get().build())
            .execute()
            .use { resp ->
                if (!resp.isSuccessful) error("HTTP ${resp.code} from $url")
                resp.body?.string().orEmpty()
            }
        parse(body, sourceUrl = url)
    }

    /** Public for unit tests. */
    fun parse(body: String, sourceUrl: String? = null): List<Profile> {
        val text = decodeIfBase64(body.trim())
        val lines = text.lineSequence()
            .map { it.trim() }
            .filter { it.isNotEmpty() && !it.startsWith("#") }
            .filter { it.startsWith("avangard://") }
            .toList()
        return lines.map { uri ->
            Profile(
                name = "",
                uri = uri,
                transport = guessTransport(uri),
                subscriptionUrl = sourceUrl,
            )
        }
    }

    private fun decodeIfBase64(s: String): String {
        // Reject anything that obviously isn't base64 so we don't garble
        // a clean newline-separated list. Heuristic: base64 has no
        // newlines / spaces / `:` characters.
        if (s.contains('\n') || s.contains(' ') || s.contains("://")) return s
        if (s.length < 8) return s
        return try {
            val flags = Base64.DEFAULT or Base64.URL_SAFE or Base64.NO_WRAP
            String(Base64.decode(s, flags), Charsets.UTF_8)
        } catch (_: Throwable) {
            s
        }
    }

    private fun guessTransport(uri: String): String {
        val q = uri.substringAfter('?', "").lowercase()
        return when {
            "transport=quic" in q || "type=quic" in q -> "quic"
            else -> "tcp"
        }
    }
}
