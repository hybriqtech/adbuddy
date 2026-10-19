package tech.hybriq.adbuddy

import android.content.Intent
import android.os.Bundle
import android.widget.Toast
import androidx.fragment.app.FragmentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.bridge.BridgeService
import tech.hybriq.adbuddy.ui.ConsentScreen
import tech.hybriq.adbuddy.ui.Onboarding
import tech.hybriq.adbuddy.ui.ChatRoot

class MainActivity : FragmentActivity() {
    private var showConsent by mutableStateOf(false)
    private var focusConfirm by mutableStateOf<String?>(null)

    /** Questions whose dialog the person closed; the notification opens them again. */
    private var closed by mutableStateOf(emptySet<String>())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val prefs = BridgePrefs(this)
        if (savedInstanceState == null) {
            handle(intent)
        } else {
            // A rotation keeps what was on screen; the consent only while the host still waits.
            showConsent = savedInstanceState.getBoolean(STATE_CONSENT) && prefs.consent == "pending"
            focusConfirm = savedInstanceState.getString(STATE_FOCUS)
            closed = savedInstanceState.getStringArrayList(STATE_CLOSED)?.toSet() ?: emptySet()
        }
        val app = application as AdbuddyApp
        // Opening the app (re)starts the channel when the host already handed over a link.
        // A rotation runs onCreate again: the service is already running and
        // Bridge.start ignores the same link, so the connection is kept.
        if (prefs.link != null) BridgeService.start(this)
        setContent {
            val dynamic by app.dynamicColor.collectAsStateWithLifecycle()
            AdbuddyTheme(dynamic = dynamic) {
                if (showConsent) {
                    ConsentScreen(
                        host = prefs.consentHost,
                        onAnswer = { accepted ->
                            prefs.consent = if (accepted) "granted" else "denied"
                            // The first-run steps follow in the app only when they have not been done: the message says what comes next.
                            val message = when {
                                !accepted -> R.string.consent_declined
                                Onboarding.of(this).done() -> R.string.consent_done
                                else -> R.string.consent_done_steps
                            }
                            Toast.makeText(this, message, Toast.LENGTH_LONG).show()
                            showConsent = false
                        },
                    )
                } else {
                    ChatRoot(app, focusConfirm, closed, onReopen = { closed = closed - it }) { closed = closed + it }
                }
            }
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putBoolean(STATE_CONSENT, showConsent)
        outState.putString(STATE_FOCUS, focusConfirm)
        outState.putStringArrayList(STATE_CLOSED, ArrayList(closed))
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent?) {
        // The consent screen only shows while the host is waiting for it.
        if (intent?.action == ACTION_SETUP && BridgePrefs(this).consent == "pending") showConsent = true
        focusConfirm = intent?.getStringExtra(EXTRA_CONFIRM)
        // Tapping the notification of a closed question opens its dialog again.
        focusConfirm?.let { closed = closed - it }
    }

    companion object {
        const val ACTION_SETUP = "tech.hybriq.adbuddy.action.SETUP"
        const val EXTRA_CONFIRM = "confirm"
        private const val STATE_CONSENT = "consent"
        private const val STATE_FOCUS = "focus"
        private const val STATE_CLOSED = "closed"
    }
}
