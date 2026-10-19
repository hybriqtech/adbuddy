package tech.hybriq.adbuddy.yolo

import android.content.Context
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity

/** The biometric half of YoloAuth (task A2): thin, with no logic of its own, so it is validated on the emulator (T9) and by hand. */
object YoloBiometric {
    /** Whether the phone has strong biometrics set up (fingerprint or face of the strong class). */
    fun strongAvailable(context: Context): Boolean =
        BiometricManager.from(context).canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS

    /**
     * Shows the prompt with BIOMETRIC_STRONG only (so no device credential: the negative button cancels) and calls
     * [onResult] once: true on success, false on an error, a cancel or a lock-out. A read that does not match
     * (onAuthenticationFailed) leaves the prompt up and is not a result.
     */
    fun prompt(activity: FragmentActivity, title: String, subtitle: String, cancel: String, onResult: (Boolean) -> Unit) {
        val prompt = BiometricPrompt(
            activity,
            ContextCompat.getMainExecutor(activity),
            object : BiometricPrompt.AuthenticationCallback() {
                override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) = onResult(true)
                override fun onAuthenticationError(errorCode: Int, errString: CharSequence) = onResult(false)
            },
        )
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setSubtitle(subtitle)
            .setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_STRONG)
            .setNegativeButtonText(cancel)
            .setConfirmationRequired(true)
            .build()
        prompt.authenticate(info)
    }
}
