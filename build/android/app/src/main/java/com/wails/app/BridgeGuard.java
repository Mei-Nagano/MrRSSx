package com.wails.app;

import android.util.Log;
import android.util.Base64;
import android.webkit.WebView;

import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;

import java.security.SecureRandom;
import java.util.Collections;

/**
 * Keeps the native bridge out of frames the app did not serve.
 *
 * {@code addJavascriptInterface} installs the object into every frame of the
 * WebView and offers no way to scope it. A document-start script removes it from
 * every frame except the app's own top document and wraps the surviving object with
 * a per-WebView capability. The capability is also checked in Java, so the same
 * boundary holds when document-start scripts are unavailable on an old WebView.
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
    static final String TRUSTED_ORIGIN = "http://wails.localhost";

    private BridgeGuard() {}

    static String newCapability() {
        byte[] bytes = new byte[32];
        new SecureRandom().nextBytes(bytes);
        return Base64.encodeToString(bytes, Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
    }

    static String documentStartScript(String capability) {
        String token = escapeJavaScript(capability);
        String origin = escapeJavaScript(TRUSTED_ORIGIN);
        return "(function(){"
                + "function drop(){try{delete window.wails;}catch(e){}"
                + "if(window.wails){window.wails=undefined;}}"
                + "var trustedPath=window.location.pathname===\"/\"||"
                + "window.location.pathname===\"/index.html\";"
                + "if(window.top!==window||window.location.origin!==\"" + origin
                + "\"||window.location.port!==\"\"||!trustedPath){drop();return;}"
                + "if(window.__mrrssBridgeWrapped){return;}"
                + "var nativeBridge=window.wails;"
                + "if(!nativeBridge){return;}"
                + "var token=\"" + token + "\";"
                + "window.__mrrssBridgeWrapped=true;"
                + "window.wails={invoke:function(payload){return nativeBridge.invoke(payload,token);},"
                + "invokeAsync:function(id,payload){return nativeBridge.invokeAsync(id,payload,token);},"
                + "log:function(level,message){return nativeBridge.log(level,message,token);},"
                + "platform:function(){return nativeBridge.platform(token);},"
                + "isDebug:function(){return nativeBridge.isDebug(token);}};"
                + "})();";
    }

    static String documentStartScript() {
        return documentStartScript("test-capability");
    }

    private static String escapeJavaScript(String value) {
        if (value == null) {
            return "";
        }
        return value.replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\r", "\\r")
                .replace("\n", "\\n");
    }

    static boolean isTrustedAppPath(String path) {
        return "/".equals(path) || "/index.html".equals(path);
    }

    /**
     * Applies the guard. The Java-side capability check in WailsJSBridge is the
     * fallback for WebViews without document-start support or when installation
     * fails; callers must still install the capability wrapper in the app document.
     */
    static void install(WebView webView, String capability) {
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            Log.w(TAG, "document-start scripts unsupported; Java capability gate is active");
            return;
        }
        try {
            WebViewCompat.addDocumentStartJavaScript(
                    webView, documentStartScript(capability), Collections.singleton(ALLOWED_ORIGIN_RULE));
        } catch (RuntimeException e) {
            Log.w(TAG, "failed to install bridge guard; Java capability gate is active", e);
        }
    }
}
