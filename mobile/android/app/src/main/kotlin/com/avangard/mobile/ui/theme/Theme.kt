package com.avangard.mobile.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import com.avangard.mobile.data.ThemeMode

private val DarkColors = darkColorScheme(
    primary = Color(0xFFB59CFF),
    secondary = Color(0xFFCBC2DB),
    tertiary = Color(0xFFEFB8C8),
)

private val LightColors = lightColorScheme(
    primary = Color(0xFF5C40B3),
    secondary = Color(0xFF625B71),
    tertiary = Color(0xFF7D5260),
)

private val AmoledColors = darkColorScheme(
    primary = Color(0xFFB59CFF),
    secondary = Color(0xFFCBC2DB),
    tertiary = Color(0xFFEFB8C8),
    background = Color.Black,
    surface = Color.Black,
    surfaceVariant = Color(0xFF101010),
)

@Composable
fun AvangardTheme(
    mode: ThemeMode = ThemeMode.SYSTEM,
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val systemDark = isSystemInDarkTheme()
    val darkTheme = when (mode) {
        ThemeMode.SYSTEM -> systemDark
        ThemeMode.LIGHT -> false
        ThemeMode.DARK, ThemeMode.AMOLED -> true
    }
    val ctx = LocalContext.current
    val colors = when {
        mode == ThemeMode.AMOLED -> AmoledColors
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (darkTheme) dynamicDarkColorScheme(ctx) else dynamicLightColorScheme(ctx)
        darkTheme -> DarkColors
        else -> LightColors
    }
    MaterialTheme(colorScheme = colors, content = content)
}
