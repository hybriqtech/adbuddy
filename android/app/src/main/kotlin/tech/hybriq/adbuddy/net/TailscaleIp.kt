package tech.hybriq.adbuddy.net

object TailscaleIp {
    /** First IPv4 address in 100.64.0.0/10 (Tailscale's CGNAT range), or "". */
    fun pick(addresses: List<String>): String = addresses.firstOrNull { inRange(it) } ?: ""

    private fun inRange(s: String): Boolean {
        val parts = s.split('.')
        if (parts.size != 4) return false
        val n = parts.map { p ->
            if (p.isEmpty() || !p.all { it in '0'..'9' }) return false
            p.toIntOrNull()?.takeIf { it in 0..255 } ?: return false
        }
        return n[0] == 100 && n[1] in 64..127
    }
}

/**
 * Decides when the Tailscale IP goes to the host: when it changes and again
 * on each welcome, never empty (an empty one means "no change" to the host).
 * Callbacks of the network come on binder threads, hence the lock.
 */
class IpReporter(private val send: (String) -> Unit) {
    private val lock = Any()
    private var current = ""
    private var sent = ""

    fun onAddresses(addresses: List<String>) {
        val ip = TailscaleIp.pick(addresses)
        val toSend = synchronized(lock) {
            current = ip
            if (ip.isNotEmpty() && ip != sent) { sent = ip; ip } else null
        }
        toSend?.let(send)
    }

    /** The VPN went away: the next IP (even the same one) is announced again. */
    fun onLost() = synchronized(lock) { current = ""; sent = "" }

    fun onWelcome() {
        val ip = synchronized(lock) { current.also { if (it.isNotEmpty()) sent = it } }
        if (ip.isNotEmpty()) send(ip)
    }
}
