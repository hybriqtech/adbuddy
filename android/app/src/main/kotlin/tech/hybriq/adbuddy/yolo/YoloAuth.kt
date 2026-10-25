package tech.hybriq.adbuddy.yolo

import android.content.Intent

/**
 * The last step to turn YOLO on (spec 2026-10-18, "se confirma con el desbloqueo del móvil"; ruling R22): strong
 * biometrics if the phone has them (BiometricPrompt with BIOMETRIC_STRONG only: the shell can neither inject nor deduce
 * it), else the lock screen credential (KeyguardManager.createConfirmDeviceCredentialIntent: PIN, pattern or password;
 * an agent that watches the key events could learn a PIN: the accepted risk), and with neither (no secure lock) a
 * specific warning, armed, that the person accepts (R20: their decision; the injected tap could then finish the activation).
 * An injected tap cannot pass the first two: the agent could otherwise leave a process running that opens ADBuddy and presses
 * "until the session ends" after the mode expired. The dialog's own checks (YoloUi.mayConfirm) come before; only an OK
 * sends the request.
 */
class YoloAuth(
    /** BiometricManager.canAuthenticate(BIOMETRIC_STRONG) == BIOMETRIC_SUCCESS, and an activity that can show the prompt. */
    private val strongBiometric: () -> Boolean,
    /** KeyguardManager.createConfirmDeviceCredentialIntent: null when the phone has no secure lock. */
    private val credentialIntent: () -> Intent?,
    private val send: (YoloUi.Choice) -> Unit,
    /** No secure lock at all: shows the warning (YoloNoLockDialog); its armed button calls [onResult] true, Cancel false. */
    private val noLock: () -> Unit,
) {
    private var waiting: YoloUi.Choice? = null

    /**
     * Starts the authentication of [choice]: [promptBiometric] shows the biometric prompt (its result comes to
     * [onResult]), or [launchCredential] starts the system's credential screen (its result comes to [onResult] too), or
     * [noLock] shows the warning of a phone with no secure lock (its answer comes to [onResult] as well).
     */
    fun start(choice: YoloUi.Choice, promptBiometric: () -> Unit, launchCredential: (Intent) -> Unit) {
        waiting = null
        if (strongBiometric()) {
            waiting = choice
            promptBiometric()
            return
        }
        val intent = credentialIntent()
        waiting = choice
        if (intent == null) {
            noLock()
            return
        }
        launchCredential(intent)
    }

    /** The authentication ended: ok is a success. A result counts once, and only after a start. */
    fun onResult(ok: Boolean) {
        val choice = waiting
        waiting = null
        if (ok && choice != null) send(choice)
    }
}
