package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class SecondaryActionButtonTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    @Test fun itIsAButtonThatClicks() {
        var clicks = 0
        compose.setContent { AdbuddyTheme(dynamic = false) { SecondaryActionButton("Not now", onClick = { clicks++ }) } }
        val node = compose.onNodeWithText("Not now")
        node.assertIsEnabled()
        assertEquals(Role.Button, compose.onNodeWithText("Not now").fetchSemanticsNode().config.getOrNull(SemanticsProperties.Role))
        node.performClick()
        assertTrue("one click", clicks == 1)
    }
}
