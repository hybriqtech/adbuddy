package tech.hybriq.adbuddy.ui

import androidx.compose.runtime.Composable

/** Stub (RED): the real guard comes next. */
internal class ObscuredCounts<K : Any>(private val read: (K) -> Boolean, private val write: (K, Boolean) -> Unit) {
    fun enter(key: K) {}
    fun exit(key: K) {}
}

@Composable
fun ObscuredTouchGuard() {}
