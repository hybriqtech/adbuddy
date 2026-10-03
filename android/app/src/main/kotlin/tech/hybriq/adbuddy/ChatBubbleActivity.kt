package tech.hybriq.adbuddy

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import tech.hybriq.adbuddy.ui.ChatRoot
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The chat inside a bubble (Bubbles API, spec M3 section 7.4). It shows no consent screen nor first-run steps
 * (those are MainActivity's), but Settings is reachable from the chat menu, as in the app, and from there the
 * notification access screen and Route.PROTECTED_APPS. Accepted explicitly: the things there that widen what the
 * agent may read are the confirm button and unticking a protected app (which also shows the agent more), and both
 * are a tap by the person. ADBuddy is a protected app, so the host's guard blocks the agent while ADBuddy is in front.
 */
class ChatBubbleActivity : ComponentActivity() {
    /** Questions whose dialog the person closed: without this, a closed 🔴 dialog would come straight back. */
    private var closed by mutableStateOf(emptySet<String>())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        // A rotation keeps the questions the person closed (MainActivity does the same).
        closed = savedInstanceState?.getStringArrayList(STATE_CLOSED)?.toSet() ?: emptySet()
        val app = application as AdbuddyApp
        setContent { AdbuddyTheme { ChatRoot(app, focusConfirm = null, closed = closed, firstRun = false, terminalOpenByDefault = false, onClose = { closed = closed + it }) } }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putStringArrayList(STATE_CLOSED, ArrayList(closed))
    }

    private companion object {
        const val STATE_CLOSED = "closed"
    }
}
