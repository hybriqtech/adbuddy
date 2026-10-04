package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/** The "not now" next to a filled primary button: outline and text strong enough to read on any theme, the same on every screen. */
@Composable
fun SecondaryActionButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    OutlinedButton(onClick = onClick, modifier = modifier, border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline)) {
        Text(text, color = MaterialTheme.colorScheme.onSurface)
    }
}
