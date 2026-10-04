package tech.hybriq.adbuddy.screens

import android.content.ComponentName
import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsNodeInteractionsProvider
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.text.TextLayoutResult
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Assert.assertTrue
import org.junit.rules.ExternalResource
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.ui.TOP_BAR_TAG

/** The phone the screens are drawn on: a Pixel-sized portrait screen, at 1.5x so the golden images stay small. */
const val PHONE = "w411dp-h891dp-hdpi"

/**
 * The ways each screen is looked at: light, dark and text at 200 % (validation 27), and text at 200 % in Spanish, the
 * app's other language and the one with the longest texts. The default language of the tests is English.
 */
enum class ScreenMode(val qualifiers: String, val fontScale: Float) {
    LIGHT("+notnight", 1f),
    DARK("+night", 1f),
    FONT_200("+notnight", 2f),
    FONT_200_ES("+es-rES-notnight", 2f),
}

/** For ParameterizedRobolectricTestRunner: one run per mode. */
fun screenModes(): List<Array<Any>> = ScreenMode.entries.map { arrayOf<Any>(it) }

/**
 * Puts the mode in the configuration before the activity starts (outer rule), so the theme (isSystemInDarkTheme)
 * and the text (the system's non-linear font scaling) read it as on a phone.
 */
class ScreenModeRule(private val mode: ScreenMode) : ExternalResource() {
    override fun before() {
        RuntimeEnvironment.setQualifiers(mode.qualifiers)
        RuntimeEnvironment.setFontScale(mode.fontScale)
    }
}

/**
 * createComposeRule() starts an empty androidx ComponentActivity, which the app's manifest does not declare.
 * Robolectric is told about it here, instead of adding ui-test-manifest (an extra activity in the debug APK).
 */
class ComponentActivityRule : ExternalResource() {
    override fun before() {
        val app = RuntimeEnvironment.getApplication()
        shadowOf(app.packageManager).addActivityIfNotPresent(ComponentName(app, ComponentActivity::class.java))
    }
}

/**
 * A golden image (src/test/screenshots/<name>_<mode>.png). -Padbuddy.screens=record records it; on Windows every other
 * run compares it and fails when any pixel changes by more than Roborazzi's default tolerance (0.7 % of colour per pixel,
 * no share of pixels allowed to change); elsewhere it is only compared (app/build.gradle.kts). Rendering is
 * Robolectric's native graphics, with the fonts of its own Android jar: nothing comes from the host.
 */
fun SemanticsNodeInteractionsProvider.shot(name: String, mode: ScreenMode) {
    onRoot().captureRoboImage("src/test/screenshots/${name}_${mode.name.lowercase()}.png")
}

/**
 * Known UI faults that the screen tests show and are not fixed yet (they belong to the app): per mode, the string
 * resources of the titles that do not fit the top bar. The check lets them through and fails once one fits, so the
 * entry goes with the fix. None today: AdbuddyTopBar closed the one there was (access_title at 200 % in Spanish).
 */
val KNOWN_UI_ISSUES: Map<ScreenMode, Set<Int>> get() = emptyMap()

fun knownUiIssue(id: Int, mode: ScreenMode): Boolean = id in KNOWN_UI_ISSUES[mode].orEmpty()

/**
 * The title must not stick out of the top bar that is drawn (the node tagged TOP_BAR_TAG, not a constant): Material 3's
 * TopAppBar does not clip its title, and no other check sees it (the text itself is whole and touches nothing). known:
 * the title is in KNOWN_UI_ISSUES, so it must still stick out (a failure tells to remove the entry).
 */
fun SemanticsNodeInteractionsProvider.assertTitleFitsTheTopBar(title: String, known: Boolean = false) {
    val node = onAllNodes(hasText(title), useUnmergedTree = true).fetchSemanticsNodes().minBy { it.positionInRoot.y }
    val barNode = onNodeWithTag(TOP_BAR_TAG, useUnmergedTree = true).fetchSemanticsNode()
    val bar = barNode.positionInRoot.y + barNode.size.height
    val bottom = node.positionInRoot.y + node.size.height
    if (known) assertTrue("known issue gone: \"$title\" fits the top bar now; remove it from KNOWN_UI_ISSUES", bottom > bar + 0.5f)
    else assertTrue("title sticks out of the top bar: \"$title\" ends at $bottom px, the bar at $bar px", bottom <= bar + 0.5f)
}

private fun SemanticsNode.texts(): List<String> = config.getOrNull(SemanticsProperties.Text).orEmpty().map { it.text }

private fun SemanticsNode.layout(): TextLayoutResult? {
    val action = config.getOrNull(SemanticsActions.GetTextLayoutResult)?.action ?: return null
    val results = mutableListOf<TextLayoutResult>()
    return if (action(results)) results.firstOrNull() else null
}

private fun SemanticsNode.withDescendants(): List<SemanticsNode> = listOf(this) + children.flatMap { it.withDescendants() }

/**
 * didOverflowWidth is no use here: a Text that wraps its content is narrower than the width it was laid out in.
 * Cut short is a line with an ellipsis, lines that do not fit the height, or a line wider than the text's box.
 */
private fun cutShort(layout: TextLayoutResult): Boolean =
    layout.didOverflowHeight || (0 until layout.lineCount).any { layout.isLineEllipsized(it) || layout.getLineRight(it) > layout.size.width + 0.5f }

/**
 * What a person would see as broken text, over every text on screen (the unmerged tree): a text cut short, a text
 * that leaves the screen sideways or above its top, or two texts drawn over each other. boundsInRoot is clipped by the
 * scrolling containers, so a text scrolled out of view (or under a bottom bar) does not count as overlapping.
 * scrolled: after a scroll a list moves texts above its top on purpose, so the top is not checked.
 */
fun SemanticsNodeInteractionsProvider.assertTextsFit(scrolled: Boolean = false) {
    val root = onNode(isRoot(), useUnmergedTree = true).fetchSemanticsNode()
    val width = root.boundsInRoot.width
    val texts = root.withDescendants().filter { it.texts().isNotEmpty() && it.layout() != null }
    assertTrue("no text on screen", texts.isNotEmpty())
    for (node in texts) {
        val text = node.texts().joinToString()
        assertTrue("cut short: \"$text\"", !cutShort(node.layout()!!))
        // Unclipped: boundsInRoot would be cut to the screen and hide it.
        val left = node.positionInRoot.x
        val right = left + node.size.width
        assertTrue("off screen sideways: \"$text\" from $left to $right (screen width $width)", left >= -0.5f && right <= width + 0.5f)
        if (scrolled) continue
        val top = node.positionInRoot.y
        assertTrue("off screen at the top: \"$text\" at $top", top >= -0.5f)
    }
    for (i in texts.indices) for (j in i + 1 until texts.size) {
        val a = texts[i].boundsInRoot
        val b = texts[j].boundsInRoot
        val overlap = a.intersect(b)
        assertTrue(
            "overlap: \"${texts[i].texts().joinToString()}\" $a and \"${texts[j].texts().joinToString()}\" $b",
            overlap.width <= 0.5f || overlap.height <= 0.5f,
        )
    }
}
