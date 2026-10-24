package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/**
 * Guards the extension mapping that keeps proxied stylesheets, module scripts and
 * fonts from being served as text/html (which makes the WebView drop them).
 */
public class ResourceMimeTest {

    @Test
    public void mapsWebAssetsThePlatformFilenameMapMisses() {
        assertEquals("text/css", ResourceMime.fromUrl("https://cdn.example.org/app.css"));
        assertEquals("text/javascript", ResourceMime.fromUrl("https://cdn.example.org/app.mjs"));
        assertEquals("text/javascript", ResourceMime.fromUrl("https://cdn.example.org/app.js"));
        assertEquals("application/json", ResourceMime.fromUrl("https://cdn.example.org/data.json"));
        assertEquals("font/woff2", ResourceMime.fromUrl("https://cdn.example.org/font.woff2"));
        assertEquals("font/woff", ResourceMime.fromUrl("https://cdn.example.org/font.woff"));
        assertEquals("font/ttf", ResourceMime.fromUrl("https://cdn.example.org/font.ttf"));
        assertEquals("font/otf", ResourceMime.fromUrl("https://cdn.example.org/font.otf"));
        assertEquals("image/svg+xml", ResourceMime.fromUrl("https://cdn.example.org/icon.svg"));
        assertEquals("image/webp", ResourceMime.fromUrl("https://cdn.example.org/photo.webp"));
        assertEquals("image/avif", ResourceMime.fromUrl("https://cdn.example.org/photo.avif"));
        assertEquals("application/wasm", ResourceMime.fromUrl("https://cdn.example.org/mod.wasm"));
        assertEquals("application/xml", ResourceMime.fromUrl("https://cdn.example.org/feed.xml"));
    }

    @Test
    public void ignoresQueryAndFragment() {
        assertEquals("text/css", ResourceMime.fromUrl("https://cdn.example.org/app.css?v=2024"));
        assertEquals("font/woff2", ResourceMime.fromUrl("https://cdn.example.org/font.woff2#iefix"));
        assertEquals("text/javascript", ResourceMime.fromUrl("https://cdn.example.org/app.mjs?a=1&b=2"));
    }

    @Test
    public void matchingIsCaseInsensitive() {
        assertEquals("text/css", ResourceMime.fromUrl("https://CDN.EXAMPLE.ORG/APP.CSS"));
        assertEquals("font/woff2", ResourceMime.fromUrl("https://cdn.example.org/Font.WOFF2"));
    }

    @Test
    public void returnsNullWhenNothingRecognisesTheUrl() {
        // The caller keeps its own fallback, so "unknown" must not be guessed here.
        assertNull(ResourceMime.fromUrl("https://example.org/a-page-without-extension"));
        assertNull(ResourceMime.fromUrl(null));
        assertNull(ResourceMime.fromUrl(""));
    }

    @Test
    public void acceptsAValidTypeHint() {
        assertEquals("text/css", ResourceMime.fromTypeHint("text/css"));
        assertEquals("text/javascript", ResourceMime.fromTypeHint("text/javascript"));
        assertEquals("text/javascript",
                ResourceMime.fromContentType("text/javascript; charset=utf-8"));
        assertEquals("application/wasm", ResourceMime.fromContentType(" application/wasm "));
    }

    @Test
    public void rejectsAnythingThatIsNotAType() {
        assertNull(ResourceMime.fromTypeHint(null));
        assertNull(ResourceMime.fromTypeHint(""));
        assertNull(ResourceMime.fromTypeHint("  "));
        assertNull(ResourceMime.fromTypeHint("html"));
        assertNull(ResourceMime.fromTypeHint("text/css<script>"));
        assertNull(ResourceMime.fromContentType("not-a-content-type"));
    }

    @Test
    public void decodesUpstreamTypeAndExtensionlessModuleBody() {
        byte[] payload = ("{\"content_type\":\"text/javascript; charset=utf-8\","
                + "\"body\":\"ZXhwb3J0IGNvbnN0IGFuc3dlciA9IDQyOw==\"}")
                .getBytes(java.nio.charset.StandardCharsets.UTF_8);
        ResourceMetadata metadata = ResourceMetadata.decode(payload);

        assertEquals("text/javascript", metadata.mimeType);
        assertEquals("export const answer = 42;",
                new String(metadata.body, java.nio.charset.StandardCharsets.UTF_8));
    }

    @Test
    public void fallsBackForLegacyByteOnlyResponses() {
        assertNull(ResourceMetadata.decode("export const answer = 42;"
                .getBytes(java.nio.charset.StandardCharsets.UTF_8)));
    }

    @Test
    public void keepsBodyWhenUpstreamTypeIsUnknownForByteFallback() {
        byte[] payload = ("{\"content_type\":\"not-a-type\","
                + "\"body\":\"aGVsbG8=\"}")
                .getBytes(java.nio.charset.StandardCharsets.UTF_8);
        ResourceMetadata metadata = ResourceMetadata.decode(payload);

        assertNull(metadata.mimeType);
        assertEquals("hello", new String(metadata.body, java.nio.charset.StandardCharsets.UTF_8));
    }
}
