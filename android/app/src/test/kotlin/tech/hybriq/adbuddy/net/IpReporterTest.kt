package tech.hybriq.adbuddy.net

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class IpReporterTest {
    private val sent = mutableListOf<String>()
    private val r = IpReporter<String> { sent += it }

    @Test fun sendsAChange() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun doesNotRepeatTheSameIp() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onAddresses("n1", listOf("198.51.100.4", "100.64.0.5"))
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun sendsTheNewIp() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onAddresses("n1", listOf("100.101.102.103"))
        assertEquals(listOf("100.64.0.5", "100.101.102.103"), sent)
    }
    @Test fun neverSendsEmpty() {
        r.onAddresses("n1", listOf("198.51.100.4"))
        r.onAddresses("n1", emptyList())
        r.onWelcome()
        assertEquals(emptyList<String>(), sent)
    }
    @Test fun lostVpnThenWelcomeSendsNothing() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onLost("n1")
        r.onWelcome()
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun welcomeResendsTheKnownIp() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onWelcome()
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
    @Test fun sameIpAfterLossIsSentAgain() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onLost("n1")
        r.onAddresses("n1", listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
    @Test fun addressesThenWelcomeThenAddressesDoesNotResend() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onWelcome()
        r.onAddresses("n1", listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
    @Test fun lossThenADifferentIpIsSent() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onLost("n1")
        r.onAddresses("n1", listOf("100.101.102.103"))
        assertEquals(listOf("100.64.0.5", "100.101.102.103"), sent)
    }
    @Test fun losingAnotherNetworkKeepsTheCurrentIp() {
        r.onAddresses("old", listOf("100.64.0.5"))
        r.onAddresses("new", listOf("100.101.102.103"))
        r.onLost("old") // a late loss of the old network must not wipe the new one
        r.onWelcome()
        assertEquals(listOf("100.64.0.5", "100.101.102.103", "100.101.102.103"), sent)
    }
    @Test fun losingTheCurrentNetworkFallsBackToTheOneThatRemains() {
        r.onAddresses("a", listOf("100.64.0.5"))
        r.onAddresses("b", listOf("100.101.102.103"))
        r.onLost("b")
        assertEquals(listOf("100.64.0.5", "100.101.102.103", "100.64.0.5"), sent)
        r.onWelcome()
        assertEquals("100.64.0.5", sent.last())
    }
    @Test fun aVpnThatComesBackWithTheSameIpIsAnnouncedAgain() {
        r.onAddresses("n1", listOf("100.64.0.5"))
        r.onAddresses("n1", listOf("198.51.100.4")) // its entry goes, sent stays
        r.onLost("n1") // no entry, no IPs left: forget what was sent
        r.onAddresses("n1", listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
    @Test fun aNetworkThatLosesItsTailscaleAddressIsForgotten() {
        r.onAddresses("a", listOf("100.64.0.5"))
        r.onAddresses("a", listOf("198.51.100.4"))
        r.onWelcome()
        assertEquals(listOf("100.64.0.5"), sent)
    }

    @Test fun theSendIsQueuedInOrderEvenWithAConcurrentChange() {
        // send runs while the reporter's lock is held: a change from another
        // thread cannot get its send queued before the one in progress.
        val order = mutableListOf<String>()
        var other: Thread? = null
        var blockedDuringSend = false
        lateinit var rep: IpReporter<String>
        rep = IpReporter { ip ->
            order += ip
            if (ip == "100.64.0.5") {
                other = Thread { rep.onAddresses("n2", listOf("100.101.102.103")) }.also { it.start() }
                other!!.join(300)
                blockedDuringSend = other!!.isAlive
            }
        }
        rep.onAddresses("n1", listOf("100.64.0.5"))
        other!!.join(5000)
        assertTrue(blockedDuringSend)
        assertEquals(listOf("100.64.0.5", "100.101.102.103"), order)
    }
}
