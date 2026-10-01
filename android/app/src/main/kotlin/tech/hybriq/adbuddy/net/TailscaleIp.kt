package tech.hybriq.adbuddy.net

object TailscaleIp {
    /** First IPv4 address in 100.64.0.0/10 (Tailscale's CGNAT range), or "". */
    fun pick(addresses: List<String>): String = addresses.firstOrNull { inRange(it) } ?: ""

    private fun inRange(s: String): Boolean {
        val parts = s.split('.')
        if (parts.size != 4) return false
        val n = parts.map { p ->
            // Plain decimal only: no sign, no spaces, no leading zeros ("0100", "064").
            if (p.isEmpty() || !p.all { it in '0'..'9' } || p.length > 1 && p[0] == '0') return false
            p.toIntOrNull()?.takeIf { it in 0..255 } ?: return false
        }
        return n[0] == 100 && n[1] in 64..127
    }
}

/**
 * Decides when the Tailscale IP goes to the host: when it changes and again
 * on each welcome, never empty (an empty one means "no change" to the host).
 * These calls come from the network-callback and socket threads, hence the lock.
 *
 * It follows each network (K is Network in the app) that has a Tailscale IP:
 * losing one only takes out its own entry, and the current IP is the one of
 * the networks that remain (the most recently updated). send runs inside the
 * lock so two sends cannot be queued in the opposite order of their decisions;
 * it must be quick and not call back into the reporter (it only queues).
 */
class IpReporter<K : Any>(private val send: (String) -> Unit) {
    private val lock = Any()
    private val ips = LinkedHashMap<K, String>() // in order of last update
    private var sent = ""

    private fun current() = ips.values.lastOrNull().orEmpty()

    fun onAddresses(network: K, addresses: List<String>) = synchronized(lock) {
        val ip = TailscaleIp.pick(addresses)
        ips.remove(network)
        if (ip.isNotEmpty()) ips[network] = ip
        announceIfNew()
    }

    /** A network went away: its IP goes with it; what remains (if anything) is announced if it is new. */
    fun onLost(network: K) = synchronized(lock) {
        ips.remove(network)
        if (ips.isEmpty()) sent = "" // the next IP (even the same one) is announced again
        else announceIfNew()
    }

    fun onWelcome() = synchronized(lock) {
        val ip = current()
        if (ip.isNotEmpty()) {
            sent = ip
            send(ip)
        }
    }

    private fun announceIfNew() {
        val ip = current()
        if (ip.isNotEmpty() && ip != sent) {
            sent = ip
            send(ip)
        }
    }
}
