package tech.hybriq.adbuddy.notify

import tech.hybriq.adbuddy.bridge.SessionState

/**
 * "Done" (spec M3 §7.4): the agent goes from working to waiting or idle after
 * at least minWorkMs of work. Pure: the caller gives the time.
 */
class DoneDetector(private val minWorkMs: Long = 10_000) {
    private var since: Long? = null

    /** True when this state ends a stretch of work long enough (once). */
    fun onState(s: SessionState, nowMs: Long): Boolean = when (s.state) {
        "working" -> {
            if (since == null) since = nowMs
            false
        }
        "waiting", "idle" -> {
            val t = since
            since = null
            t != null && nowMs - t >= minWorkMs
        }
        else -> { // stopped or unknown: whatever was going on is over
            since = null
            false
        }
    }

    /** The connection was lost: what was seen no longer counts. */
    fun reset() { since = null }
}
