package tech.hybriq.adbuddy.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/** Success and warning, which Material 3 lacks: the status pill and the journal's levels (spec §4.1). */
@Immutable
data class AdbuddyColors(
    val success: Color, val onSuccess: Color, val successContainer: Color, val onSuccessContainer: Color,
    val warning: Color, val onWarning: Color, val warningContainer: Color, val onWarningContainer: Color,
) {
    companion object {
        fun of(m: Map<String, Int>) = AdbuddyColors(
            Color(m.getValue("success")), Color(m.getValue("onSuccess")), Color(m.getValue("successContainer")), Color(m.getValue("onSuccessContainer")),
            Color(m.getValue("warning")), Color(m.getValue("onWarning")), Color(m.getValue("warningContainer")), Color(m.getValue("onWarningContainer")),
        )
        val Light = of(Palette.extraLight)
        val Dark = of(Palette.extraDark)
    }
}

val LocalAdbuddyColors = staticCompositionLocalOf { AdbuddyColors.Light }
