package tech.hybriq.adbuddy.a11y

import tech.hybriq.adbuddy.bridge.BridgeState

/**
 * What the YOLO mode changes in what the helpers protect (spec 2026-10-18, ruling R1 of the plan): the user's protected
 * apps and the heads-up notification block (a 2FA code) stop applying; ADBuddy itself is always protected, or the agent
 * could reach the switch that turns the mode on. Nothing here is a way to turn the mode on: it only reads its state.
 */
object YoloProtection {
    /**
     * Whether the helpers behave as under YOLO: the host said it is on, and the link is up. With the link down, or the
     * state not known, the state may be stale (the host may have turned it off): everything protects as before.
     */
    fun yoloOn(state: BridgeState): Boolean = state.connection == BridgeState.Connection.CONNECTED && state.yolo?.active == true

    /** The predicate UiExecutor gets for "is this package protected": ADBuddy always, the user's list unless YOLO is on. */
    fun effective(yoloActive: () -> Boolean, userProtected: (String) -> Boolean): (String) -> Boolean =
        { pkg -> pkg == Packages.ADBUDDY || !yoloActive() && userProtected(pkg) }

    /** Whether a notification on screen blocks the helpers: not under YOLO. */
    fun notificationBlocks(yoloActive: () -> Boolean, onScreen: () -> Boolean): () -> Boolean =
        { !yoloActive() && onScreen() }
}
