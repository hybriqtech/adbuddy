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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview

private fun Map<String, Int>.c(name: String) = Color(getValue(name))

private val lightScheme: ColorScheme = Palette.light.let {
    lightColorScheme(
        primary = it.c("primary"), onPrimary = it.c("onPrimary"), primaryContainer = it.c("primaryContainer"), onPrimaryContainer = it.c("onPrimaryContainer"),
        secondary = it.c("secondary"), onSecondary = it.c("onSecondary"), secondaryContainer = it.c("secondaryContainer"), onSecondaryContainer = it.c("onSecondaryContainer"),
        background = it.c("background"), onBackground = it.c("onBackground"), surface = it.c("surface"), onSurface = it.c("onSurface"),
        surfaceVariant = it.c("surfaceVariant"), onSurfaceVariant = it.c("onSurfaceVariant"), outline = it.c("outline"),
        error = it.c("error"), onError = it.c("onError"), errorContainer = it.c("errorContainer"), onErrorContainer = it.c("onErrorContainer"),
    )
}

private val darkScheme: ColorScheme = Palette.dark.let {
    darkColorScheme(
        primary = it.c("primary"), onPrimary = it.c("onPrimary"), primaryContainer = it.c("primaryContainer"), onPrimaryContainer = it.c("onPrimaryContainer"),
        secondary = it.c("secondary"), onSecondary = it.c("onSecondary"), secondaryContainer = it.c("secondaryContainer"), onSecondaryContainer = it.c("onSecondaryContainer"),
        background = it.c("background"), onBackground = it.c("onBackground"), surface = it.c("surface"), onSurface = it.c("onSurface"),
        surfaceVariant = it.c("surfaceVariant"), onSurfaceVariant = it.c("onSurfaceVariant"), outline = it.c("outline"),
        error = it.c("error"), onError = it.c("onError"), errorContainer = it.c("errorContainer"), onErrorContainer = it.c("onErrorContainer"),
    )
}

/** Material 3, light and dark, with Material You on Android 12+ and ADBuddy's palette elsewhere (spec M3 §7.1). */
@Composable
fun AdbuddyTheme(dark: Boolean = isSystemInDarkTheme(), dynamic: Boolean = true, content: @Composable () -> Unit) {
    val context = LocalContext.current
    val scheme = when {
        dynamic && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> darkScheme
        else -> lightScheme
    }
    MaterialTheme(colorScheme = scheme, content = content)
}

/** Every screen has these three previews: light, dark and text at 200 % (spec M3 §7.1). */
@Preview(name = "light", showBackground = true)
@Preview(name = "dark", uiMode = Configuration.UI_MODE_NIGHT_YES, showBackground = true)
@Preview(name = "font 200%", fontScale = 2f, showBackground = true)
annotation class AdbuddyPreviews
