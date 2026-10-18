package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.time.Instant
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.ui.ConfirmDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The 🔴 dialog in every ScreenMode: the command in a monospaced block of its own, inside the one watched scroll
 * (spec §8.2): a second scroll in the command block would make "read to the end" not cover it.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmDialogScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    // 300+ characters, with quotes and pipes.
    private val cmd = "adb shell \"dumpsys package com.example.app | grep -i 'permission' | sort | uniq\" && " +
        (1..6).joinToString(" && ") { "adb shell 'settings put global stay_on_while_plugged_in $it | cat'" }

    private fun show(raw: String, special: Boolean = false) {
        val r = ConfirmRequest(
            id = "j_1", title = "Restart the phone", detail = "", rawCommand = raw, special = special,
            expiresAt = Instant.parse("2026-10-01T10:10:00Z").toString(),
        )
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmDialog(r, ConfirmUi.Status.OPEN, app.clock, armed = true, now = ConfirmUi.expiry(r)!! - 540_000, onClose = {}, onAnswer = {})
            }
        }
        compose.waitForIdle()
    }

    private fun checks() {
        compose.onAllNodes(hasScrollAction()).assertCountEquals(1)
        compose.onNode(hasText(cmd.take(40), substring = true) and hasAnyAncestor(hasScrollAction())).assertExists()
    }

    @Test fun theCommandIsAMonospacedBlockInsideTheOneWatchedScroll() {
        show(cmd)
        checks()
        compose.shot("confirm_dialog", mode)
    }

    @Test fun theSpecialWarningComesFirstAboveTheBlock() {
        // The host flags it special (spec §8.6): the warning is shown first, above the command block.
        show(cmd, special = true)
        compose.onAllNodes(hasScrollAction()).assertCountEquals(1)
        compose.onNodeWithText(app.getString(R.string.confirm_special), substring = true).assertExists()
        compose.shot("confirm_dialog_special", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
