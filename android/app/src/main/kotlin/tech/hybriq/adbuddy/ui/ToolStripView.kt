package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.ToolActivity
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** What the agent is doing with its tools now (ToolStrip.of): one small chip per tool, with its state in words. */
@Composable
fun ToolStripView(tools: List<ToolActivity>, modifier: Modifier = Modifier) {
    if (tools.isEmpty()) return
    Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
        for (t in tools) {
            Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.small) {
                Row(Modifier.padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (t.status == "running") CircularProgressIndicator(Modifier.size(12.dp), strokeWidth = 2.dp)
                    val state = when (t.status) {
                        "running" -> R.string.tool_running
                        "error" -> R.string.tool_error
                        else -> R.string.tool_done
                    }
                    Text(t.summary, Modifier.weight(1f, fill = false), style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    Text(
                        stringResource(state), style = MaterialTheme.typography.labelSmall,
                        color = if (t.status == "error") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@AdbuddyPreviews
@Composable
private fun ToolStripPreview() {
    AdbuddyTheme(dynamic = false) {
        ToolStripView(listOf(ToolActivity("t1", "", "Read build.gradle", "done"), ToolActivity("t2", "", "Run the tests", "running")), Modifier.padding(12.dp))
    }
}
