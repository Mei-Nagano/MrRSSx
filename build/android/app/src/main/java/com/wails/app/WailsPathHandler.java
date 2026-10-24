package com.wails.app;

import android.net.Uri;
import android.util.Log;
import android.webkit.WebResourceResponse;

import androidx.annotation.NonNull;
import androidx.annotation.Nullable;
import androidx.webkit.WebViewAssetLoader;

import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/**
 * WailsPathHandler implements WebViewAssetLoader.PathHandler to serve assets
 * from the Go asset server. This allows the WebView to load assets without
 * using a network server, similar to iOS's WKURLSchemeHandler.
 */
public class WailsPathHandler implements WebViewAssetLoader.PathHandler {
    private static final String TAG = "WailsPathHandler";
    private static final boolean DEBUG = BuildConfig.DEBUG;

    private final WailsBridge bridge;
    private final String bridgeCapability;

    public WailsPathHandler(WailsBridge bridge, String bridgeCapability) {
        this.bridge = bridge;
        this.bridgeCapability = bridgeCapability;
    }

    @Nullable
    @Override
    public WebResourceResponse handle(@NonNull String path) {
        if (DEBUG) Log.d(TAG, "Handling path: " + path);

        // Normalize path
        if (path.isEmpty() || path.equals("/")) {
            path = "/index.html";
        }

        // Get asset from Go
        byte[] data = bridge.serveAsset(path, "GET", "{}");

        if (data == null || data.length == 0) {
            Log.w(TAG, "Asset not found: " + path);
            return null; // Return null to let WebView handle 404
        }

        if (path.equals("/index.html")) {
            data = injectBridgeGuard(data, bridgeCapability);
        }

        // Determine MIME type
        String mimeType = bridge.getAssetMimeType(path);
        if (DEBUG) Log.d(TAG, "Serving " + path + " with type " + mimeType + " (" + data.length + " bytes)");

        // Create response
        InputStream inputStream = new ByteArrayInputStream(data);
        Map<String, String> headers = responseHeaders(path);

        return new WebResourceResponse(
                mimeType,
                "UTF-8",
                200,
                "OK",
                headers,
                inputStream
        );
    }

    static Map<String, String> responseHeaders(String path) {
        Map<String, String> headers = new HashMap<>();
        if (path == null || path.isEmpty() || "/".equals(path) || "/index.html".equals(path)) {
            // The bootstrap capability is in this document. An opaque article
            // frame must not be able to fetch and read it through CORS.
            headers.put("Cache-Control", "no-store");
            headers.put("Cross-Origin-Resource-Policy", "same-origin");
            headers.put("X-Content-Type-Options", "nosniff");
        } else {
            headers.put("Access-Control-Allow-Origin", "*");
            headers.put("Cache-Control", "no-cache");
        }
        return headers;
    }

    static byte[] injectBridgeGuard(byte[] data, String capability) {
        String html = new String(data, StandardCharsets.UTF_8);
        String lower = html.toLowerCase(java.util.Locale.ROOT);
        String guard = "<script>" + BridgeGuard.documentStartScript(capability) + "</script>";
        int headStart = lower.indexOf("<head");
        while (headStart >= 0 && headStart + 5 < lower.length()
                && Character.isLetterOrDigit(lower.charAt(headStart + 5))) {
            headStart = lower.indexOf("<head", headStart + 5);
        }
        if (headStart >= 0) {
            int headEnd = lower.indexOf('>', headStart);
            if (headEnd >= 0) {
                String guarded = html.substring(0, headEnd + 1) + guard + html.substring(headEnd + 1);
                return guarded.getBytes(StandardCharsets.UTF_8);
            }
        }
        int doctypeStart = lower.indexOf("<!doctype");
        if (doctypeStart >= 0) {
            int doctypeEnd = lower.indexOf('>', doctypeStart);
            if (doctypeEnd >= 0) {
                String guarded = html.substring(0, doctypeEnd + 1) + guard + html.substring(doctypeEnd + 1);
                return guarded.getBytes(StandardCharsets.UTF_8);
            }
        }
        return (guard + html).getBytes(StandardCharsets.UTF_8);
    }
}
