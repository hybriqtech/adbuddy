package tech.hybriq.adbuddy.net

import org.junit.Assert.assertEquals
import org.junit.Test

class TailscaleIpTest {
    @Test fun picksTheTailscaleAddress(): Unit = assertEquals("100.101.102.103", TailscaleIp.pick(listOf("198.51.100.4", "100.101.102.103")))
    @Test fun ipv6IsIgnored() = assertEquals("", TailscaleIp.pick(listOf("fd7a:115c:a1e0::1")))
    @Test fun outsideTheRange(): Unit = assertEquals("", TailscaleIp.pick(listOf("100.63.255.255", "100.128.0.0")))
    @Test fun limitsAreIncluded() {
        assertEquals("100.64.0.0", TailscaleIp.pick(listOf("100.64.0.0", "100.127.255.255")))
        assertEquals("100.127.255.255", TailscaleIp.pick(listOf("100.127.255.255")))
    }
    @Test fun firstOfTheListWins(): Unit = assertEquals("100.101.102.103", TailscaleIp.pick(listOf("100.101.102.103", "100.100.100.100")))
    @Test fun invalidEntriesAreIgnored(): Unit = assertEquals("", TailscaleIp.pick(listOf("abc", "", "100.64.0", "100.64.0.256", "100.64.0.-1", "100.64.0.5.1")))
    @Test fun invalidDoesNotHideValid(): Unit = assertEquals("100.64.0.5", TailscaleIp.pick(listOf("abc", "100.64.0.256", "100.64.0.5")))
    @Test fun noAddresses(): Unit = assertEquals("", TailscaleIp.pick(emptyList()))
}

class IpReporterTest {
    private val sent = mutableListOf<String>()
    private val r = IpReporter { sent += it }

    @Test fun sendsAChange() {
        r.onAddresses(listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun doesNotRepeatTheSameIp() {
        r.onAddresses(listOf("100.64.0.5"))
        r.onAddresses(listOf("198.51.100.4", "100.64.0.5"))
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun sendsTheNewIp() {
        r.onAddresses(listOf("100.64.0.5"))
        r.onAddresses(listOf("100.101.102.103"))
        assertEquals(listOf("100.64.0.5", "100.101.102.103"), sent)
    }
    @Test fun neverSendsEmpty() {
        r.onAddresses(listOf("198.51.100.4"))
        r.onAddresses(emptyList())
        r.onWelcome()
        assertEquals(emptyList<String>(), sent)
    }
    @Test fun lostVpnThenWelcomeSendsNothing() {
        r.onAddresses(listOf("100.64.0.5"))
        r.onLost()
        r.onWelcome()
        assertEquals(listOf("100.64.0.5"), sent)
    }
    @Test fun welcomeResendsTheKnownIp() {
        r.onAddresses(listOf("100.64.0.5"))
        r.onWelcome()
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
    @Test fun sameIpAfterLossIsSentAgain() {
        r.onAddresses(listOf("100.64.0.5"))
        r.onLost()
        r.onAddresses(listOf("100.64.0.5"))
        assertEquals(listOf("100.64.0.5", "100.64.0.5"), sent)
    }
}
