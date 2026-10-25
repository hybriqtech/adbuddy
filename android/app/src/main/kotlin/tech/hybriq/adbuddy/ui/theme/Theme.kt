package tech.hybriq.adbuddy.ui.theme

import android.content.res.Configuration
import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview

private fun Map<String, Int>.c(name: String) = Color(getValue(name))

private fun scheme(p: Map<String, Int>, dark: Boolean): ColorScheme {
    val base = if (dark) darkColorScheme() else lightColorScheme()
    return base.copy(
        primary = p.c("primary"), onPrimary = p.c("onPrimary"), primaryContainer = p.c("primaryContainer"), onPrimaryContainer = p.c("onPrimaryContainer"),
        inversePrimary = p.c("inversePrimary"),
        secondary = p.c("secondary"), onSecondary = p.c("onSecondary"), secondaryContainer = p.c("secondaryContainer"), onSecondaryContainer = p.c("onSecondaryContainer"),
        tertiary = p.c("tertiary"), onTertiary = p.c("onTertiary"), tertiaryContainer = p.c("tertiaryContainer"), onTertiaryContainer = p.c("onTertiaryContainer"),
        background = p.c("background"), onBackground = p.c("onBackground"), surface = p.c("surface"), onSurface = p.c("onSurface"),
        surfaceVariant = p.c("surfaceVariant"), onSurfaceVariant = p.c("onSurfaceVariant"), surfaceTint = p.c("surfaceTint"),
        inverseSurface = p.c("inverseSurface"), inverseOnSurface = p.c("inverseOnSurface"),
        error = p.c("error"), onError = p.c("onError"), errorContainer = p.c("errorContainer"), onErrorContainer = p.c("onErrorContainer"),
        outline = p.c("outline"), outlineVariant = p.c("outlineVariant"), scrim = p.c("scrim"),
        surfaceBright = p.c("surfaceBright"), surfaceDim = p.c("surfaceDim"),
        surfaceContainer = p.c("surfaceContainer"), surfaceContainerHigh = p.c("surfaceContainerHigh"), surfaceContainerHighest = p.c("surfaceContainerHighest"),
        surfaceContainerLow = p.c("surfaceContainerLow"), surfaceContainerLowest = p.c("surfaceContainerLowest"),
        primaryFixed = p.c("primaryFixed"), primaryFixedDim = p.c("primaryFixedDim"), onPrimaryFixed = p.c("onPrimaryFixed"), onPrimaryFixedVariant = p.c("onPrimaryFixedVariant"),
        secondaryFixed = p.c("secondaryFixed"), secondaryFixedDim = p.c("secondaryFixedDim"), onSecondaryFixed = p.c("onSecondaryFixed"), onSecondaryFixedVariant = p.c("onSecondaryFixedVariant"),
        tertiaryFixed = p.c("tertiaryFixed"), tertiaryFixedDim = p.c("tertiaryFixedDim"), onTertiaryFixed = p.c("onTertiaryFixed"), onTertiaryFixedVariant = p.c("onTertiaryFixedVariant"),
    )
}

private val lightScheme = scheme(Palette.light, dark = false)
private val darkScheme = scheme(Palette.dark, dark = true)

/**
 * Material 3 with ADBuddy's palette, light and dark. Material You (the wallpaper's colours, Android 12+) only when
 * asked (spec R6: Settings, "Use wallpaper colors"); the semantic colours (AdbuddyTheme.colors) never follow it.
 */
@Composable
fun AdbuddyTheme(dark: Boolean = isSystemInDarkTheme(), dynamic: Boolean = false, content: @Composable () -> Unit) {
    val context = LocalContext.current
    val scheme = when {
        dynamic && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> darkScheme
        else -> lightScheme
    }
    CompositionLocalProvider(LocalAdbuddyColors provides if (dark) AdbuddyColors.Dark else AdbuddyColors.Light) {
        MaterialTheme(colorScheme = scheme, typography = AdbuddyTypography, shapes = AdbuddyShapes, content = content)
    }
}

object AdbuddyTheme {
    val colors: AdbuddyColors
        @Composable @ReadOnlyComposable get() = LocalAdbuddyColors.current
}

/** Every screen has these three previews: light, dark and text at 200 % (spec M3 §7.1). */
@Preview(name = "light", showBackground = true)
@Preview(name = "dark", uiMode = Configuration.UI_MODE_NIGHT_YES, showBackground = true)
@Preview(name = "font 200%", fontScale = 2f, showBackground = true)
annotation class AdbuddyPreviews
