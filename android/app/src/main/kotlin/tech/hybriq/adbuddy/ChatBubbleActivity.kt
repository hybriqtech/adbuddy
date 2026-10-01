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

/** The chat inside a bubble (Bubbles API, spec M3 section 7.4). Opens no consent screen: that is MainActivity's. */
class ChatBubbleActivity : ComponentActivity() {
    /** Questions whose dialog the person closed: without this, a closed 🔴 dialog would come straight back. */
    private var closed by mutableStateOf(emptySet<String>())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val app = application as AdbuddyApp
        setContent { AdbuddyTheme { ChatRoot(app, focusConfirm = null, closed = closed, onClose = { closed = closed + it }) } }
    }
}
