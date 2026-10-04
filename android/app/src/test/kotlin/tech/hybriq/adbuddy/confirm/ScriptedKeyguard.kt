package tech.hybriq.adbuddy.confirm

import android.app.KeyguardManager
import org.robolectric.annotation.Implementation
import org.robolectric.annotation.Implements
import org.robolectric.shadows.ShadowKeyguardManager

/**
 * A KeyguardManager whose isDeviceLocked can be scripted reading by reading (the test of a lock that changes between
 * two reads in the same call). Without a script it behaves as ShadowKeyguardManager (setIsDeviceLocked).
 * Used with @Config(shadows = [ScriptedKeyguard::class]).
 */
@Implements(KeyguardManager::class)
class ScriptedKeyguard : ShadowKeyguardManager() {
    @Implementation
    override fun isDeviceLocked(): Boolean = script.removeFirstOrNull() ?: super.isDeviceLocked()

    companion object {
        val script = ArrayDeque<Boolean>()
    }
}
