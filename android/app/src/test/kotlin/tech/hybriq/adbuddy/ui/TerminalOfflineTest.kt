package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertAll
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.isNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.screens.ScreenMode
import tech.hybriq.adbuddy.screens.ScreenModeRule
import tech.hybriq.adbuddy.screens.assertTextsFit
import tech.hybriq.adbuddy.screens.screenModes
import tech.hybriq.adbuddy.term.QuickKey
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The quick keys rest while there is no channel to the PC. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class QuickKeysEnabledTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val keys = mutableListOf<QuickKey>()

    @Test fun enabledKeyIsSent() {
        compose.setContent { AdbuddyTheme(dynamic = false) { QuickKeysRow(onKey = { keys += it }, enabled = true) } }
        compose.onNodeWithText("Esc").performClick()
        assertEquals(listOf(QuickKey.ESC), keys)
    }

    @Test fun disabledKeysAreAllDisabledAndSendNothing() {
        compose.setContent { AdbuddyTheme(dynamic = false) { QuickKeysRow(enabled = false) { keys += it } } }
        compose.onAllNodes(hasClickAction()).assertCountEquals(QuickKey.entries.size)
        compose.onAllNodes(hasClickAction()).assertAll(isNotEnabled())
        compose.onNodeWithText("Esc").performClick()
        assertTrue(keys.isEmpty())
    }
}

/** The notice over the terminal, in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class TerminalOfflineNoticeTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    @Test fun noticeIsShownFitsAndIsAnnounced() {
        compose.setContent { AdbuddyTheme(dynamic = false) { TerminalOfflineNotice() } }
        compose.onNodeWithText(RuntimeEnvironment.getApplication().getString(R.string.terminal_offline)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.onNode(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite)).assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
