package tech.hybriq.adbuddy.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDirection

/** The system font, with titles a little heavier (spec §4.2). */
internal val AdbuddyTypography: Typography = Typography().let { t ->
    t.copy(
        titleLarge = t.titleLarge.copy(fontWeight = FontWeight.SemiBold),
        titleMedium = t.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        labelLarge = t.labelLarge.copy(fontWeight = FontWeight.Medium),
    )
}

/** What is going to run (commands, IP:port, keys): monospaced and always left to right (spec R7). */
object AdbuddyType {
    val code: TextStyle
        @Composable @ReadOnlyComposable get() = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace, textDirection = TextDirection.Ltr)
    val codeSmall: TextStyle
        @Composable @ReadOnlyComposable get() = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, textDirection = TextDirection.Ltr)
}
