package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performSemanticsAction
import org.junit.Assert.assertEquals
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.ui.YoloActivationDialog
import tech.hybriq.adbuddy.ui.YoloCard
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import tech.hybriq.adbuddy.yolo.YoloUi

/** The YOLO card of Settings and the activation dialog, in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class YoloScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)
    private fun connected(y: YoloState?) = BridgeState(connection = BridgeState.Connection.CONNECTED, yolo = y)

    private var turnedOn = 0
    private var turnedOff = 0

    private fun card(state: BridgeState) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { YoloCard(state, app.clock, onTurnOn = { turnedOn++ }, onTurnOff = { turnedOff++ }) }
        }
    }

    @Test fun offUnknownOnAndDisconnected() {
        card(connected(YoloState()))
        compose.onNodeWithText(text(R.string.yolo_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.yolo_off)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("yolo_card_off", mode)
    }

    @Test fun unknownDrawsNoSwitchToTurnOn() {
        card(connected(null))
        compose.onNodeWithText(text(R.string.yolo_unknown)).assertIsDisplayed()
        compose.onNode(isToggleable()).assertIsNotEnabled()
    }

    @Test fun disconnectedCannotTurnOn() {
        card(BridgeState(yolo = YoloState()))
        compose.onNodeWithText(text(R.string.yolo_disconnected)).assertIsDisplayed()
        compose.onNode(isToggleable()).assertIsNotEnabled()
    }

    @Test fun onWithSessionAndTurningItOffAsksNothing() {
        card(connected(YoloState(active = true, session = true)))
        compose.onNodeWithText(text(R.string.yolo_on_session)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("yolo_card_session", mode)
        compose.onNode(isToggleable()).performClick()
        assertEquals("off is one tap, with no question", 1, turnedOff)
        assertEquals(0, turnedOn)
    }

    @Test fun turnedOffTheSwitchOnlyAsks() {
        card(connected(YoloState()))
        compose.onNode(isToggleable()).performClick()
        assertEquals(1, turnedOn)
        assertEquals(0, turnedOff)
    }

    // The dialog: its clock is the test's, so the arming wait is exact.
    private var elapsed = 10_000L
    private var locked = false
    private var confirmed: YoloUi.Choice? = null

    // The clock of Compose is the test's too (no autoAdvance): the arming wait ends when the test says so.
    private fun dialog(connected: Boolean = true) {
        compose.mainClock.autoAdvance = false
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                YoloActivationDialog(connected, isLocked = { locked }, onConfirm = { confirmed = it }, onDismiss = {}, elapsed = { elapsed })
            }
        }
        settle()
    }

    /** Lets the frames after an interaction run. */
    private fun settle() = compose.mainClock.advanceTimeBy(50)

    /**
     * The phone's time and Compose's go by ConfirmUi.ARMING_MS: the button has been shown long enough. Twice, because
     * the dialog arms again each time the activity resumes (LifecycleResumeEffect), which in some ScreenModes (a
     * configuration change by the font scale) happens after the first wait started.
     */
    private fun arm() {
        repeat(2) {
            elapsed += ConfirmUi.ARMING_MS
            // One jump of Compose's clock, not 40 frames: at 200 % text each frame of the dialog costs memory.
            compose.mainClock.advanceTimeBy(ConfirmUi.ARMING_MS + 100, ignoreFrameDuration = true)
            settle()
        }
    }

    private fun pick(id: Int) {
        // The row's own click action, not a touch: at 200 % text the rows are below the fold of the dialog, and
        // scrolling to them with the clock stopped never ends (the test JVM runs out of memory).
        compose.onNodeWithText(text(id)).performSemanticsAction(SemanticsActions.OnClick)
        settle()
    }

    // This one scrolls (at 200 % text the list is longer than the dialog): the clock of Compose runs by itself, because
    // with it stopped the scroll never ends and the test JVM runs out of memory. No golden image of the dialog either:
    // capturing its second window at 200 % text is not worth it (measured).
    @Test fun theDialogSaysWhatIsSkipped() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                YoloActivationDialog(connected = true, isLocked = { false }, onConfirm = { confirmed = it }, onDismiss = {}, elapsed = { elapsed })
            }
        }
        compose.onNodeWithText(text(R.string.yolo_dialog_title)).assertIsDisplayed()
        for (id in listOf(R.string.yolo_dialog_intro, R.string.yolo_skip_confirmations, R.string.yolo_skip_blocks, R.string.yolo_skip_journal, R.string.yolo_skip_protected, R.string.yolo_skip_permissions, R.string.yolo_pause_note, R.string.yolo_auth_note, R.string.yolo_for_15, R.string.yolo_for_60, R.string.yolo_for_session)) {
            compose.onNodeWithText(text(id)).performScrollTo().assertIsDisplayed()
        }
    }

    // Mutation: give `picked` a default value.
    @Test fun thereIsNoDefaultDurationAndTheButtonIsOffUntilOneIsChosen() {
        dialog()
        arm()
        compose.onNodeWithText(text(R.string.yolo_confirm)).assertIsNotEnabled() // armed, but nothing chosen
    }

    // Mutation (each): drop the choice or the arming from mayConfirm (YoloUi).
    @Test fun nothingIsSentWithoutAChoiceAndTheArmingWait() {
        dialog()
        pick(R.string.yolo_for_15)
        // Chosen, but the button has not been shown for ARMING_MS.
        compose.onNodeWithText(text(R.string.yolo_confirm)).assertIsNotEnabled().performClick()
        assertEquals(null, confirmed)
        arm()
        compose.onNodeWithText(text(R.string.yolo_confirm)).assertIsEnabled().performClick()
        assertEquals(YoloUi.Choice.QUARTER, confirmed)
    }

    // Mutation: call onConfirm without mayConfirm in the button's onClick, or take `locked` out of mayConfirm.
    @Test fun aLockedPhoneConfirmsNothing() {
        dialog()
        pick(R.string.yolo_for_session)
        arm()
        locked = true // read again at the tap
        compose.onNodeWithText(text(R.string.yolo_confirm)).performClick()
        assertEquals(null, confirmed)
        locked = false
        compose.onNodeWithText(text(R.string.yolo_confirm)).performClick()
        assertEquals(YoloUi.Choice.SESSION, confirmed)
    }

    @Test fun withoutThePcTheDialogCannotConfirm() {
        dialog(connected = false)
        pick(R.string.yolo_for_60)
        arm()
        compose.onNodeWithText(text(R.string.yolo_confirm)).assertIsNotEnabled()
        assertEquals(null, confirmed)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
