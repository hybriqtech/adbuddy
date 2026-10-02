package tech.hybriq.adbuddy

import android.content.SharedPreferences

/**
 * Just enough of SharedPreferences for the tests of the classes that keep a few values in it (NotificationConsent;
 * see NotificationGateTest). Like the real one, an edit only takes effect on apply() or commit(); clear() goes first,
 * then the puts and removes in call order. Not modelled: change listeners, thread safety.
 */
class FakePrefs : SharedPreferences {
    val map = mutableMapOf<String, Any>()
    private val listeners = mutableListOf<SharedPreferences.OnSharedPreferenceChangeListener>()
    val listenerCount get() = listeners.size
    override fun getAll(): MutableMap<String, *> = map
    override fun getString(key: String?, defValue: String?) = map[key] as String? ?: defValue
    @Suppress("UNCHECKED_CAST")
    override fun getStringSet(key: String?, defValues: MutableSet<String>?) = map[key] as MutableSet<String>? ?: defValues
    override fun getInt(key: String?, defValue: Int) = map[key] as Int? ?: defValue
    override fun getLong(key: String?, defValue: Long) = map[key] as Long? ?: defValue
    override fun getFloat(key: String?, defValue: Float) = map[key] as Float? ?: defValue
    override fun getBoolean(key: String?, defValue: Boolean) = map[key] as Boolean? ?: defValue
    override fun contains(key: String?) = map.containsKey(key)
    override fun edit(): SharedPreferences.Editor = object : SharedPreferences.Editor {
        private val removed = Any()
        private var clear = false
        private val pending = linkedMapOf<String, Any>() // removed marks a key to delete
        override fun putString(key: String, value: String?) = apply { pending[key] = value ?: removed }
        override fun putStringSet(key: String, values: MutableSet<String>?) = apply { pending[key] = values?.toMutableSet() ?: removed }
        override fun putInt(key: String, value: Int) = apply { pending[key] = value }
        override fun putLong(key: String, value: Long) = apply { pending[key] = value }
        override fun putFloat(key: String, value: Float) = apply { pending[key] = value }
        override fun putBoolean(key: String, value: Boolean) = apply { pending[key] = value }
        override fun remove(key: String) = apply { pending[key] = removed }
        override fun clear() = apply { clear = true }
        override fun commit(): Boolean {
            val changed = linkedSetOf<String?>()
            if (clear) { changed.addAll(map.keys); map.clear() }
            for ((k, v) in pending) { if (v === removed) map.remove(k) else map[k] = v; changed.add(k) }
            for (k in changed) for (l in listeners.toList()) l.onSharedPreferenceChanged(this@FakePrefs, k)
            return true
        }
        override fun apply() { commit() }
    }
    override fun registerOnSharedPreferenceChangeListener(l: SharedPreferences.OnSharedPreferenceChangeListener?) { l?.let { listeners.add(it) } }
    override fun unregisterOnSharedPreferenceChangeListener(l: SharedPreferences.OnSharedPreferenceChangeListener?) { listeners.remove(l) }
}
