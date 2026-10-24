package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.nio.charset.StandardCharsets;

import org.junit.Test;

public class WailsPathHandlerTest {
    @Test
    public void keepsDoctypeAndRunsGuardBeforePageScripts() {
        byte[] input = "<!doctype html><html><head><script type=\"module\">boot()</script></head></html>"
                .getBytes(StandardCharsets.UTF_8);

        String output = new String(
                WailsPathHandler.injectBridgeGuard(input, "test-capability"), StandardCharsets.UTF_8);

        assertTrue(output.startsWith("<!doctype html>"));
        assertTrue(output.contains("test-capability"));
        assertTrue(output.indexOf("window.__mrrssBridgeWrapped")
                < output.indexOf("<script type=\"module\">"));
    }

    @Test
    public void keepsBootstrapCapabilityOutOfCrossOriginReadableHeaders() {
        java.util.Map<String, String> bootstrap = WailsPathHandler.responseHeaders("/index.html");
        assertNull(bootstrap.get("Access-Control-Allow-Origin"));
        assertEquals("same-origin", bootstrap.get("Cross-Origin-Resource-Policy"));
        assertEquals("nosniff", bootstrap.get("X-Content-Type-Options"));
        assertNull(WailsPathHandler.responseHeaders("/").get("Access-Control-Allow-Origin"));

        java.util.Map<String, String> asset = WailsPathHandler.responseHeaders("/assets/app.js");
        assertEquals("*", asset.get("Access-Control-Allow-Origin"));
    }
}
