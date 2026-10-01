package tech.hybriq.adbuddy.term

import android.annotation.SuppressLint
import android.graphics.Color as AColor
import android.webkit.JavascriptInterface
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.webkit.WebViewAssetLoader
import java.io.ByteArrayInputStream
import java.util.Base64

/** The three methods the page may call (spec M3 §7.3). They run on a WebView thread, not the main one. */
private class JsHost(private val bridge: TermBridge) {
    @JavascriptInterface fun data(s: String) = bridge.data(s)
    @JavascriptInterface fun size(cols: Int, rows: Int) = bridge.size(cols, rows)
    @JavascriptInterface fun link(url: String) = bridge.link(url)
}

/**
 * Draws the effects of the model in the page. What comes before the page has loaded waits, and so does the theme
 * (the first one is set before the page exists). After the renderer died it draws nothing: a new view replaces it.
 */
private class WebSink(private val web: WebView) : TermSink {
    private var ready = false
    private var dead = false
    private var theme = ""
    private val queue = ArrayList<TermEffect>()

    fun loaded() { web.post { if (dead) return@post; ready = true; draw(theme); queue.forEach(::draw); queue.clear() } }
    fun setTheme(js: String) { web.post { theme = js; if (ready && !dead) draw(js) } }
    fun die() { web.post { dead = true; queue.clear() } }

    override fun apply(e: TermEffect) {
        web.post { if (!dead) { if (ready) draw(e) else queue += e } }
    }

    private fun draw(js: String) { if (js.isNotEmpty()) web.evaluateJavascript(js, null) }

    private fun b64(b: ByteArray) = Base64.getEncoder().encodeToString(b)

    // Base64 has no quote nor backslash, so it is safe inside a JS string.
    private fun draw(e: TermEffect) = when (e) {
        is TermEffect.Reset -> draw("termReset(${e.cols},${e.rows},'${b64(e.bytes)}',${e.replay})")
        is TermEffect.Write -> draw("termWrite('${b64(e.bytes)}',${e.replay})")
        is TermEffect.Resize -> draw("termResize(${e.cols},${e.rows})")
        TermEffect.Resync -> Unit // TerminalFeed handles it
    }
}

private fun hex(argb: Int) = "#%06X".format(argb and 0xFFFFFF)

/**
 * The agent's terminal (spec M3 §7.3): xterm.js from the app's own assets,
 * in a WebView that cannot reach the network, the files or any content. It
 * watches the terminal only while it is on screen.
 */
@SuppressLint("SetJavaScriptEnabled") // xterm.js needs it; nothing else is loaded (TermWebPolicy)
@Composable
fun TerminalView(feed: TerminalFeed, onLink: (url: String, domain: String) -> Unit, modifier: Modifier = Modifier) {
    val colors = MaterialTheme.colorScheme
    val themeJs = "termTheme('${hex(colors.surface.toArgb())}','${hex(colors.onSurface.toArgb())}','${hex(colors.primary.toArgb())}')"
    // A crash of the renderer leaves the WebView unusable (onRenderProcessGone): a new key makes a new view.
    var generation by remember { mutableIntStateOf(0) }

    key(generation) {
        val holder = remember { arrayOfNulls<WebSink>(1) }
        AndroidView(
            modifier = modifier,
            factory = { context ->
                val loader = WebViewAssetLoader.Builder().addPathHandler("/assets/", WebViewAssetLoader.AssetsPathHandler(context)).build()
                val onMain = android.os.Handler(android.os.Looper.getMainLooper())
                WebView(context).apply {
                    setBackgroundColor(AColor.TRANSPARENT)
                    settings.apply {
                        javaScriptEnabled = true
                        allowFileAccess = false
                        allowContentAccess = false
                        domStorageEnabled = false
                        blockNetworkLoads = true
                        mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
                        setSupportMultipleWindows(false)
                        setGeolocationEnabled(false)
                    }
                    val bridge = TermBridge(
                        onInput = feed::input,
                        onSize = feed::resize,
                        onLink = { url, domain -> onMain.post { onLink(url, domain) } },
                    )
                    addJavascriptInterface(JsHost(bridge), "adbuddy")
                    val sink = WebSink(this)
                    holder[0] = sink
                    webViewClient = object : WebViewClient() {
                        override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                            val url = request.url.toString()
                            return (if (TermWebPolicy.allowed(url)) loader.shouldInterceptRequest(request.url) else null) ?: blocked()
                        }

                        // The page never navigates: every attempt is consumed here.
                        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest) = true
                        override fun onPageFinished(view: WebView, url: String) = sink.loaded()

                        // Returning true keeps the app alive; this view cannot be used again, so a new one takes its place.
                        override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                            sink.die()
                            feed.detach(sink)
                            view.post { generation++ }
                            return true
                        }
                    }
                    loadUrl(TermWebPolicy.PAGE)
                }
            },
            update = { holder[0]?.setTheme(themeJs) },
            onRelease = { it.destroy() },
        )

        // The stream runs only while the view is on screen (protocol spec §7).
        LifecycleStartEffect(feed, generation) {
            val sink = holder[0]
            if (sink != null) feed.attach(sink)
            onStopOrDispose { if (sink != null) feed.detach(sink) }
        }
    }
}

private fun blocked() = WebResourceResponse("text/plain", "utf-8", 403, "Blocked", emptyMap(), ByteArrayInputStream(ByteArray(0)))
