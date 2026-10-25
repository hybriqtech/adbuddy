package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R

/** An empty conversation: the logo, what to do, and three suggestions that fill the field (they never send by themselves). */
@Composable
fun ChatEmptyState(paused: Boolean, onSuggestion: (String) -> Unit, modifier: Modifier = Modifier) {
    // The Box centres; the Column inside scrolls (a scrolling Column gets unbounded height and would not centre itself).
    Box(modifier, contentAlignment = Alignment.Center) { EmptyColumn(paused, onSuggestion) }
}

@Composable
private fun EmptyColumn(paused: Boolean, onSuggestion: (String) -> Unit) {
    // Scrolls: at 200 % text the logo, the line and three chips do not fit a short screen. No covered-touch guard: a
    // suggestion only fills the field (spec §8.5).
    Column(Modifier.verticalScroll(rememberScrollState()).padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Image(
            painterResource(R.drawable.ic_launcher_monochrome), contentDescription = null, modifier = Modifier.size(72.dp),
            colorFilter = ColorFilter.tint(MaterialTheme.colorScheme.primary),
        )
        Text(
            stringResource(if (paused) R.string.chat_empty_paused else R.string.chat_empty_ready),
            style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
        )
        if (!paused) {
            for (id in listOf(R.string.suggestion_battery, R.string.suggestion_volume, R.string.suggestion_screenshot)) {
                val s = stringResource(id)
                SuggestionChip(onClick = { onSuggestion(s) }, label = { Text(s) })
            }
        }
    }
}
