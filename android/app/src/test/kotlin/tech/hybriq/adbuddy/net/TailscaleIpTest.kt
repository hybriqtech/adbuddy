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
    @Test fun invalidEntriesAreIgnored(): Unit = assertEquals("", TailscaleIp.pick(listOf("abc", "", "100.64.0", "100.64.0.256", "100.64.0.-1", "100.64.0.5.1", "0100.64.0.1", "100.064.0.1", "100.64.00.1", "100.64.0.01", "+100.64.0.1", " 100.64.0.1")))
    @Test fun invalidDoesNotHideValid(): Unit = assertEquals("100.64.0.5", TailscaleIp.pick(listOf("abc", "100.64.0.256", "100.64.0.5")))
    @Test fun noAddresses(): Unit = assertEquals("", TailscaleIp.pick(emptyList()))
}

