package com.avangard.mobile.data

import kotlinx.serialization.Serializable
import java.util.UUID

@Serializable
data class Profile(
    val id: String = UUID.randomUUID().toString(),
    val name: String,
    val uri: String,
    val transport: String = "tcp",
    val createdAt: Long = System.currentTimeMillis(),
    /** Source subscription URL if this profile was imported from one. */
    val subscriptionUrl: String? = null,
) {
    /** A short display name fall-through (host:port) for unnamed profiles. */
    val displayName: String
        get() = name.ifBlank { uri.substringAfter("@").substringBefore("?") }
}
