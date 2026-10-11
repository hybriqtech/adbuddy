package tech.hybriq.adbuddy.screens

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus
import tech.hybriq.adbuddy.ui.DraftChips
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The attachment chips (preparing, uploading, failed), in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class DraftChipsScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    @Test fun preparingUploadingAndFailed() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                Surface(Modifier.fillMaxSize()) {
                    Column(Modifier.padding(vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        DraftChips(listOf(Draft("a", "a.jpg", "image/jpeg", 0, 0, DraftStatus.UPLOADING)), {}, {})
                        DraftChips(listOf(Draft("b", "b.jpg", "image/jpeg", 1000, 400, DraftStatus.UPLOADING)), {}, {})
                        DraftChips(listOf(Draft("c", "c.txt", "text/plain", 10, 0, DraftStatus.FAILED)), {}, {})
                    }
                }
            }
        }
        compose.assertTextsFit()
        compose.shot("draft_chips", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
