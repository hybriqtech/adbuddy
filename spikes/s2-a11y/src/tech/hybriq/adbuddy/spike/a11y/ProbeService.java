package tech.hybriq.adbuddy.spike.a11y;

import android.accessibilityservice.AccessibilityService;
import android.util.Log;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;

/** Spike desechable: registra en logcat cuándo se conecta y cuánto cuesta leer el árbol de cada ventana. */
public class ProbeService extends AccessibilityService {
    private static final String TAG = "A11yProbe";

    @Override
    protected void onServiceConnected() {
        Log.i(TAG, "CONNECTED");
    }

    @Override
    public void onAccessibilityEvent(AccessibilityEvent event) {
        if (event.getEventType() != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) return;
        long t0 = System.nanoTime();
        AccessibilityNodeInfo root = getRootInActiveWindow();
        int nodes = root == null ? -1 : countNodes(root);
        long ms = (System.nanoTime() - t0) / 1_000_000;
        Log.i(TAG, "WINDOW pkg=" + event.getPackageName() + " nodes=" + nodes + " ms=" + ms);
    }

    private static int countNodes(AccessibilityNodeInfo node) {
        int total = 1;
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i);
            if (child != null) total += countNodes(child);
        }
        return total;
    }

    @Override
    public void onInterrupt() {
        Log.i(TAG, "INTERRUPT");
    }
}
