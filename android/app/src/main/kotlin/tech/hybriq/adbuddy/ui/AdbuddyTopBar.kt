package tech.hybriq.adbuddy.ui

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R

const val TOP_BAR_TAG = "adbuddy_top_bar"

/** Material 3's bar is 64 dp and does not clip its title: when two lines of title need more room (large text), the bar takes it. */
object AdbuddyTopBarSpec {
    val NORMAL: Dp = 64.dp

    fun height(twoLines: Dp): Dp = maxOf(NORMAL, twoLines + 8.dp)
}

/** The one top bar of the screens that have a back arrow: the title is a heading (TalkBack) and takes up to two lines. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AdbuddyTopBar(title: String, onBack: () -> Unit) {
    val style = MaterialTheme.typography.titleLarge
    val twoLines = with(LocalDensity.current) { (style.lineHeight * 2).toDp() }
    TopAppBar(
        title = { Text(title, style = style, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.semantics { heading() }) },
        navigationIcon = {
            IconButton(onClick = onBack) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.action_back))
            }
        },
        modifier = Modifier.testTag(TOP_BAR_TAG),
        expandedHeight = AdbuddyTopBarSpec.height(twoLines),
    )
}
