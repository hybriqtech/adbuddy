package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp

/** From this text size on, the footer's actions are stacked: two buttons in a row would squeeze each other's words. */
const val STACK_FROM_FONT_SCALE = 1.3f

/**
 * The actions at the foot of a step: in a row at the end, or (large text) stacked at full width with the main one on top.
 * Each action receives the Modifier it has to take (full width when stacked).
 */
@Composable
fun ActionFooter(
    main: @Composable (Modifier) -> Unit,
    other: (@Composable (Modifier) -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    if (LocalDensity.current.fontScale > STACK_FROM_FONT_SCALE) {
        Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            main(Modifier.fillMaxWidth())
            other?.invoke(Modifier.fillMaxWidth())
        }
    } else {
        Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End), verticalAlignment = Alignment.CenterVertically) {
            other?.invoke(Modifier)
            main(Modifier)
        }
    }
}
