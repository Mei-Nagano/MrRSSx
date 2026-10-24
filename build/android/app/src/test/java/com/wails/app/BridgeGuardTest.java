package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * The bridge is injected into every frame and cannot be scoped by the platform. The
 * document-start wrapper and Java capability gate together keep the native API out
 * of article frames, including on WebViews without document-start support.
 */
public class BridgeGuardTest {

    @Test
    public void keepsTheBridgeInTheTopDocument() {
        String script = BridgeGuard.documentStartScript();
        assertTrue(script.contains("window.top!==window"));
        assertTrue(script.contains("window.location.origin!==\"http://wails.localhost\""));
        assertTrue(script.contains("window.location.pathname===\"/\""));
        assertTrue(script.contains("window.location.port!==\"\""));
        assertTrue(script.contains("window.__mrrssBridgeWrapped=true"));
    }

    @Test
    public void dropsTheBridgeInOtherFrames() {
        String script = BridgeGuard.documentStartScript();
        assertTrue("deletion attempt", script.contains("delete window.wails"));
        assertTrue("explicit clear", script.contains("window.wails=undefined"));
    }

    @Test
    public void clearsOnlyAfterTryingToDelete() {
        String script = BridgeGuard.documentStartScript();
        int drop = script.indexOf("delete window.wails");
        int clear = script.indexOf("window.wails=undefined");
        assertTrue("a native-injected property may refuse deletion", drop >= 0 && clear > drop);
    }

    @Test
    public void appliesToEveryFrameIncludingOpaqueOnes() {
        // The article iframe is sandboxed without allow-same-origin, so its origin is
        // opaque. An origin rule naming the app origin would skip exactly the frames
        // the guard exists to strip.
        assertEquals("*", BridgeGuard.ALLOWED_ORIGIN_RULE);
    }

    @Test
    public void wrapsCallsWithCapabilityForOldWebViews() {
        String script = BridgeGuard.documentStartScript("capability");
        assertTrue(script.contains("nativeBridge.invoke(payload,token)"));
        assertTrue(script.contains("nativeBridge.invokeAsync(id,payload,token)"));
        assertTrue(script.contains("nativeBridge.platform(token)"));
        assertTrue(!script.contains("mrrssToken"));
    }

    @Test
    public void rejectsMissingOrWrongCapability() {
        assertTrue(WailsJSBridge.isAuthorizedCapability("secret", "secret"));
        assertTrue(!WailsJSBridge.isAuthorizedCapability(null, "secret"));
        assertTrue(!WailsJSBridge.isAuthorizedCapability("other", "secret"));
    }

    @Test
    public void rejectsSameOriginProxyAsAnAppDocument() {
        assertTrue(BridgeGuard.isTrustedAppPath("/"));
        assertTrue(BridgeGuard.isTrustedAppPath("/index.html"));
        assertTrue(!BridgeGuard.isTrustedAppPath("/api/webpage/resource"));
        assertTrue(!BridgeGuard.isTrustedAppPath("/article.html"));
    }
}
