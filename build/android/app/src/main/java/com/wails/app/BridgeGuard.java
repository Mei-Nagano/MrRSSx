package com.wails.app;

import android.util.Log;
import android.webkit.WebView;

import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;

import java.util.Collections;

/**
 * Keeps the native bridge out of frames the app did not serve.
 *
 * {@code addJavascriptInterface} installs the object into every frame of the
 * WebView and offers no way to scope it, so an article page rendered in a sandboxed
 * iframe could reach the same native API the app uses. A document-start script runs
 * in each frame before any page script; this one drops the object in every frame
 * that is not the top document, which is the only document the app itself serves.
 *
 * The script is a plain string so it can be asserted without an Android runtime.
 */
final class BridgeGuard {

    private static final String TAG = "WailsBridgeGuard";

    /**
     * Frames are matched against this rule before the script runs. It has to be the
     * wildcard: the frames that must lose the bridge are the sandboxed article
     * frames, which run in an opaque origin and would never match the app origin.
     */
    static final String ALLOWED_ORIGIN_RULE = "*";

    private BridgeGuard() {}

    static String documentStartScript() {
        return "(function(){"
                + "if(window.top===window){return;}"
                + "try{delete window.wails;}catch(e){}"
                + "if(window.wails){window.wails=undefined;}"
                + "})();";
    }

    /**
     * Applies the guard. Without document-start support the platform gives no hook
     * that runs before page scripts, so the bridge stays reachable from frames; that
     * is logged rather than silently assumed to be in place.
     */
    static void install(WebView webView) {
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            Log.w(TAG, "document-start scripts unsupported; cannot scope the bridge to the app frame");
            return;
        }
        try {
            WebViewCompat.addDocumentStartJavaScript(
                    webView, documentStartScript(), Collections.singleton(ALLOWED_ORIGIN_RULE));
        } catch (RuntimeException e) {
            Log.w(TAG, "failed to install bridge guard", e);
        }
    }
}
