package com.wails.app;

import java.net.URLConnection;
import java.util.Locale;

/**
 * Maps a proxied resource's original URL to the MIME type the WebView needs.
 *
 * <p>The bridge returns response bytes only, so the upstream {@code Content-Type}
 * never reaches the host and the type has to be inferred. The original URL is the
 * same fact the proxy used to fetch the resource, which makes its extension a
 * better signal than a guess. Stylesheets, module scripts and fonts carry no
 * magic bytes, so inferring them from the payload is impossible.
 *
 * <p>Kept free of Android types so it can be unit tested on a plain JVM.
 */
final class ResourceMime {

    private ResourceMime() {
    }

    /**
     * Returns the MIME type for {@code url}, or {@code null} when nothing
     * recognises it. The caller keeps its own last-resort fallback.
     */
    static String fromUrl(String url) {
        if (url == null || url.isEmpty()) {
            return null;
        }
        String clean = stripQuery(url);
        if (clean.isEmpty()) {
            return null;
        }
        String lower = clean.toLowerCase(Locale.ROOT);

        // Explicit table first: these are the web assets that the platform's
        // built-in filename map either misses or reports inconsistently.
        if (lower.endsWith(".css")) return "text/css";
        if (lower.endsWith(".mjs") || lower.endsWith(".js")) return "text/javascript";
        if (lower.endsWith(".json")) return "application/json";
        if (lower.endsWith(".woff2")) return "font/woff2";
        if (lower.endsWith(".woff")) return "font/woff";
        if (lower.endsWith(".ttf")) return "font/ttf";
        if (lower.endsWith(".otf")) return "font/otf";
        if (lower.endsWith(".svg")) return "image/svg+xml";
        if (lower.endsWith(".webp")) return "image/webp";
        if (lower.endsWith(".avif")) return "image/avif";
        if (lower.endsWith(".wasm")) return "application/wasm";
        if (lower.endsWith(".xml")) return "application/xml";

        return URLConnection.guessContentTypeFromName(clean);
    }

    private static String stripQuery(String url) {
        int cut = url.indexOf('?');
        String withoutQuery = cut >= 0 ? url.substring(0, cut) : url;
        int hash = withoutQuery.indexOf('#');
        return hash >= 0 ? withoutQuery.substring(0, hash) : withoutQuery;
    }

    /**
     * Accepts the type hint the proxy carries in the URL. The proxy knows the
     * resource is a stylesheet even when its URL has no extension, and the bytes
     * alone cannot tell CSS from an HTML fragment.
     */
    static String fromTypeHint(String hint) {
        if (hint == null) {
            return null;
        }
        String trimmed = hint.trim();
        if (trimmed.isEmpty() || trimmed.length() > 100) {
            return null;
        }
        if (!trimmed.matches("[a-z0-9.+-]+/[a-z0-9.+-]+")) {
            return null;
        }
        return trimmed;
    }
}
