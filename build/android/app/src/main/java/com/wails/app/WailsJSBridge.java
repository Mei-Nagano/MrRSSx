package com.wails.app;

import android.util.Log;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.Locale;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;
import com.wails.app.BuildConfig;

/**
 * WailsJSBridge provides the JavaScript interface that allows the web frontend
 * to communicate with the Go backend. This is exposed to JavaScript as the
 * `window.wails` object.
 *
 * Similar to iOS's WKScriptMessageHandler but using Android's addJavascriptInterface.
 */
public class WailsJSBridge {
    private static final String TAG = "WailsJSBridge";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    // Pooled threads avoid unbounded thread creation under high call volume.
    private static final ExecutorService executor = Executors.newCachedThreadPool();

    private final WailsBridge bridge;
    private final WebView webView;
    private final String capability;

    public WailsJSBridge(WailsBridge bridge, WebView webView, String capability) {
        this.bridge = bridge;
        this.webView = webView;
        this.capability = capability;
    }

    /**
     * Send a message to Go and return the response synchronously.
     * Called from JavaScript: wails.invoke(message, capability)
     *
     * @param message The message to send (JSON string)
     * @return The response from Go (JSON string)
     */
    @JavascriptInterface
    public String invoke(String message, String suppliedCapability) {
        if (!isAuthorized(suppliedCapability)) {
            return "{\"error\":\"unauthorized bridge call\"}";
        }
        if (DEBUG) Log.d(TAG, "Invoke called: " + message);
        return bridge.handleMessage(message);
    }

    /**
     * Send a message to Go asynchronously.
     * The response will be sent back via a callback.
     * Called from JavaScript: wails.invokeAsync(callbackId, message, capability)
     *
     * @param callbackId The callback ID to use for the response
     * @param message The message to send (JSON string)
     */
    @JavascriptInterface
    public void invokeAsync(final String callbackId, final String payload,
                            final String suppliedCapability) {
        if (!isAuthorized(suppliedCapability)) {
            // Do not invoke a callback owned by the main document on behalf of
            // an untrusted frame. The guarded app wrapper always supplies the
            // capability, so rejection can return without touching WebView JS.
            return;
        }
        if (DEBUG) Log.d(TAG, "InvokeAsync called: " + payload);

        // Handle off the JS thread so we don't block the WebView.
        executor.execute(() -> {
            try {
                String response = bridge.handleRuntimeCall(payload);
                sendCallback(callbackId, response, null);
            } catch (Exception e) {
                Log.e(TAG, "Error in async invoke", e);
                sendCallback(callbackId, null, e.getMessage());
            }
        });
    }

    /** Log a message from the trusted frontend without exposing an ungated method. */
    @JavascriptInterface
    public void log(String level, String message, String suppliedCapability) {
        if (!isAuthorized(suppliedCapability)) return;
        String safeLevel = level == null ? "" : level.toLowerCase(Locale.ROOT);
        switch (safeLevel) {
            case "debug": Log.d(TAG + "/JS", message); break;
            case "info": Log.i(TAG + "/JS", message); break;
            case "warn": Log.w(TAG + "/JS", message); break;
            case "error": Log.e(TAG + "/JS", message); break;
            default: Log.v(TAG + "/JS", message); break;
        }
    }

    @JavascriptInterface
    public String platform(String suppliedCapability) {
        return isAuthorized(suppliedCapability) ? "android" : "";
    }

    @JavascriptInterface
    public boolean isDebug(String suppliedCapability) {
        return isAuthorized(suppliedCapability) && DEBUG;
    }

    /**
     * Send a callback response to JavaScript
     */
    private void sendCallback(String callbackId, String result, String error) {
        final String js;
        if (error != null) {
            js = String.format(
                    "window._wailsAndroidCallback && window._wailsAndroidCallback('%s', null, '%s');",
                    escapeJsString(callbackId),
                    escapeJsString(error)
            );
        } else {
            js = String.format(
                    "window._wailsAndroidCallback && window._wailsAndroidCallback('%s', '%s', null);",
                    escapeJsString(callbackId),
                    escapeJsString(result != null ? result : "")
            );
        }

        webView.post(() -> webView.evaluateJavascript(js, null));
    }

    /**
     * The native object is visible to every WebView frame. Only the app's
     * top-document wrapper knows this capability; frames that bypass the wrapper
     * (including old WebViews without document-start scripts) are rejected here.
     */
    static boolean isAuthorizedCapability(String suppliedCapability, String expectedCapability) {
        if (suppliedCapability == null || expectedCapability == null || expectedCapability.isEmpty()) {
            return false;
        }
        return expectedCapability.equals(suppliedCapability);
    }

    private boolean isAuthorized(String suppliedCapability) {
        return isAuthorizedCapability(suppliedCapability, capability);
    }

    private String escapeJsString(String str) {
        if (str == null) return "";
        return str.replace("\\", "\\\\")
                .replace("'", "\\'")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                // JS line terminators (U+2028/U+2029) must be escaped too; built via
                // (char) casts so the Java lexer does not reinterpret them as newlines.
                .replace(String.valueOf((char) 0x2028), "\\u2028")
                .replace(String.valueOf((char) 0x2029), "\\u2029");
    }
}
