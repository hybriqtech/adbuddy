package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.ProgressBarRangeInfo
import androidx.compose.ui.test.hasProgressBarRangeInfo
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class DraftChipsTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private fun s(id: Int): String = RuntimeEnvironment.getApplication().getString(id)

    private val removed = mutableListOf<String>()
    private val retried = mutableListOf<String>()

    private fun show(d: Draft) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { DraftChips(listOf(d), { removed += it }, { retried += it }) }
        }
    }

    @Test fun aFileBeingPreparedShowsAnIndeterminateProgress() {
        show(Draft("k", "a.jpg", "image/jpeg", 0, 0, DraftStatus.UPLOADING))
        compose.onNode(hasProgressBarRangeInfo(ProgressBarRangeInfo.Indeterminate)).assertExists()
    }

    @Test fun anUploadShowsItsFraction() {
        show(Draft("k", "a.jpg", "image/jpeg", 1000, 400, DraftStatus.UPLOADING))
        compose.onNode(hasProgressBarRangeInfo(ProgressBarRangeInfo.Indeterminate)).assertDoesNotExist()
        compose.onNode(hasProgressBarRangeInfo(ProgressBarRangeInfo(0.4f, 0f..1f))).assertExists()
    }

    @Test fun theRemoveButtonIsFingerSizedAndRemoves() {
        show(Draft("k", "a.jpg", "image/jpeg", 1000, 400, DraftStatus.UPLOADING))
        val node = compose.onNodeWithContentDescription(s(R.string.chat_discard))
        val size = node.fetchSemanticsNode().size
        val density = RuntimeEnvironment.getApplication().resources.displayMetrics.density
        assertTrue("width ${size.width / density} dp", size.width / density >= 48f - 0.5f)
        assertTrue("height ${size.height / density} dp", size.height / density >= 48f - 0.5f)
        node.performClick()
        assertEquals(listOf("k"), removed)
    }

    @Test fun tappingAFailedChipRetriesButTheXOnlyRemoves() {
        show(Draft("k", "a.jpg", "image/jpeg", 10, 0, DraftStatus.FAILED))
        compose.onNodeWithText(s(R.string.draft_failed)).performClick()
        assertEquals(listOf("k"), retried)
        compose.onNodeWithContentDescription(s(R.string.chat_discard)).performClick()
        assertEquals(listOf("k"), removed)
        assertEquals(listOf("k"), retried)
    }
}
