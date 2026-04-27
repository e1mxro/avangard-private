package com.avangard.mobile.data

import kotlinx.serialization.Serializable

enum class TunnelMode { SYSTEM_VPN, SOCKS5_ONLY }

enum class ThemeMode { SYSTEM, LIGHT, DARK, AMOLED }

/**
 * How [perAppPackages] is interpreted when starting the VPN.
 *
 * - [ALL]: the VPN routes traffic from every app on the device.
 * - [ALLOW]: only the listed packages are routed; everything else bypasses.
 * - [DISALLOW]: every app is routed except the listed packages.
 */
enum class PerAppMode { ALL, ALLOW, DISALLOW }

@Serializable
data class AppSettings(
    val activeProfileId: String? = null,
    val tunnelMode: String = TunnelMode.SYSTEM_VPN.name,
    val themeMode: String = ThemeMode.SYSTEM.name,
    val perAppMode: String = PerAppMode.ALL.name,
    val perAppPackages: List<String> = emptyList(),
) {
    fun tunnelModeEnum(): TunnelMode = enumValueOrDefault(tunnelMode, TunnelMode.SYSTEM_VPN)
    fun themeModeEnum(): ThemeMode = enumValueOrDefault(themeMode, ThemeMode.SYSTEM)
    fun perAppModeEnum(): PerAppMode = enumValueOrDefault(perAppMode, PerAppMode.ALL)
}

private inline fun <reified T : Enum<T>> enumValueOrDefault(name: String, default: T): T =
    runCatching { enumValueOf<T>(name) }.getOrDefault(default)
