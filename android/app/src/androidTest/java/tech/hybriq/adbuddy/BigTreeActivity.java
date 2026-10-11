package tech.hybriq.adbuddy;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.view.ViewGroup;
import android.widget.LinearLayout;
import android.widget.TextView;

/**
 * Only for ForegroundHeadsOnEmulatorTest: a window with a tree of about "views" nested views (extra, default 2000),
 * each one important for accessibility, so the accessibility tree has them all (a RecyclerView or a Settings list
 * exposes only the visible rows). Fan-out 4, so the depth stays near 7 for 2000 and 9 for 9000.
 *
 * Java and exported for the same reasons as TapjackOverlayActivity: it runs in the tests' own package and
 * process, which carries no Kotlin stdlib, and the instrumentation starts it with startActivitySync.
 */
public class BigTreeActivity extends Activity {
    private static final int FANOUT = 4;

    @Override protected void onCreate(Bundle b) {
        super.onCreate(b);
        int views = getIntent().getIntExtra("views", 2000);
        setContentView(build(views), new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
    }

    /** A subtree of exactly n views (n >= 1). */
    private View build(int n) {
        if (n <= 1) {
            TextView t = new TextView(this);
            t.setText("x");
            t.setTextSize(4);
            t.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_YES);
            return t;
        }
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_YES);
        int rest = n - 1;
        int kids = Math.min(FANOUT, rest);
        for (int i = 0; i < kids; i++) {
            int share = rest / kids + (i < rest % kids ? 1 : 0);
            box.addView(build(share));
        }
        return box;
    }
}
