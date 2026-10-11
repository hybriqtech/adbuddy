package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

/** What is wrong with the link, in full, under the header: the text may carry a command to run, so it is never cut. */
@Composable
fun LinkBanner(status: LinkStatus, modifier: Modifier = Modifier) {
    if (!status.bannerWorthy) return
    val scheme = MaterialTheme.colorScheme
    // A broken or cut link is an error; waiting, paused and not set up are states to act on, not faults.
    val problem = status == LinkStatus.CUT || status == LinkStatus.UPDATE_APP || status == LinkStatus.UPDATE_HOST
    Surface(
        color = if (problem) scheme.errorContainer else scheme.secondaryContainer,
        contentColor = if (problem) scheme.onErrorContainer else scheme.onSecondaryContainer,
        modifier = modifier.fillMaxWidth().semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
    ) {
        Text(stringResource(status.text), style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
    }
}
