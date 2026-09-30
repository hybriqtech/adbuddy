package tech.hybriq.adbuddy

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.bridge.BridgeService
import tech.hybriq.adbuddy.ui.ConsentScreen
import tech.hybriq.adbuddy.ui.HomeScreen

class MainActivity : ComponentActivity() {
    private var showConsent by mutableStateOf(false)
    private var focusConfirm by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handle(intent)
        // Asked once per launch, not again on every rotation.
        if (savedInstanceState == null && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        val app = application as AdbuddyApp
        val prefs = BridgePrefs(this)
        // Opening the app (re)starts the channel when the host already handed over a link.
        // A rotation runs onCreate again: the service is already running and
        // Bridge.start ignores the same link, so the connection is kept.
        if (prefs.link != null) BridgeService.start(this)
        setContent {
            MaterialTheme {
                val state by app.bridge.state.collectAsStateWithLifecycle()
                if (showConsent) {
                    ConsentScreen(
                        host = prefs.consentHost,
                        onAnswer = { accepted ->
                            prefs.consent = if (accepted) "granted" else "denied"
                            if (accepted) Toast.makeText(this, R.string.consent_done, Toast.LENGTH_LONG).show()
                            showConsent = false
                        },
                    )
                } else {
                    HomeScreen(state = state, bridge = app.bridge, focusConfirm = focusConfirm)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent?) {
        // The consent screen only shows while the host is waiting for it.
        if (intent?.action == ACTION_SETUP && BridgePrefs(this).consent == "pending") showConsent = true
        focusConfirm = intent?.getStringExtra(EXTRA_CONFIRM)
    }

    companion object {
        const val ACTION_SETUP = "tech.hybriq.adbuddy.action.SETUP"
        const val EXTRA_CONFIRM = "confirm"
    }
}
