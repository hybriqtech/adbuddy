package tech.hybriq.adbuddy.net

import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.util.Log
import java.util.concurrent.Executors
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.DeviceInfo
import tech.hybriq.adbuddy.bridge.Protocol

/**
 * Tells the host this phone's Tailscale IP in device.info (M5b). protectedApps
 * stays null: the host reads that as "no change", so UiHelpers' own device.info
 * is not overwritten.
 */
class DeviceInfoSender(private val app: AdbuddyApp) {
    private val work = Executors.newSingleThreadExecutor { Thread(it, "adbuddy-deviceinfo") }
    private val reporter = IpReporter { ip -> work.execute { sendIp(ip) } }
    private var callback: ConnectivityManager.NetworkCallback? = null
    private var unlisten: (() -> Unit)? = null

    private fun sendIp(ip: String) {
        try {
            val info = DeviceInfo(tailscaleIp = ip)
            app.bridge.send(Protocol.DEVICE_INFO, Protocol.json.encodeToJsonElement(DeviceInfo.serializer(), info))
        } catch (e: RuntimeException) {
            Log.w(TAG, "device.info", e)
        }
    }

    @Synchronized
    fun start() {
        if (callback != null) return
        try {
            val cm = app.getSystemService(ConnectivityManager::class.java) ?: return
            val cb = object : ConnectivityManager.NetworkCallback() {
                override fun onLinkPropertiesChanged(network: Network, linkProperties: LinkProperties) = report(linkProperties)
                override fun onLost(network: Network) = reporter.onLost()
            }
            // The builder adds NOT_VPN by default; without removing it nothing matches a VPN.
            val request = NetworkRequest.Builder()
                .addTransportType(NetworkCapabilities.TRANSPORT_VPN)
                .removeCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
                .build()
            cm.registerNetworkCallback(request, cb)
            callback = cb
            unlisten = app.bridge.listen(Protocol.WELCOME) { reporter.onWelcome() }
        } catch (e: SecurityException) {
            Log.w(TAG, "network callback", e)
        } catch (e: RuntimeException) {
            Log.w(TAG, "network callback", e)
        }
    }

    @Synchronized
    fun stop() {
        unlisten?.invoke()
        unlisten = null
        val cb = callback ?: return
        callback = null
        try {
            app.getSystemService(ConnectivityManager::class.java)?.unregisterNetworkCallback(cb)
        } catch (e: RuntimeException) {
            Log.w(TAG, "unregister", e)
        }
    }

    private fun report(lp: LinkProperties) {
        reporter.onAddresses(lp.linkAddresses.mapNotNull { it.address.hostAddress })
    }

    private companion object {
        const val TAG = "ADBuddyDeviceInfo"
    }
}
