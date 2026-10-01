package tech.hybriq.adbuddy

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import tech.hybriq.adbuddy.ui.ChatRoot
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The chat inside a bubble (Bubbles API, spec M3 section 7.4). Opens no consent screen: that is MainActivity's. */
class ChatBubbleActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val app = application as AdbuddyApp
        setContent { AdbuddyTheme { ChatRoot(app, focusConfirm = null, closed = emptySet(), onClose = {}) } }
    }
}
