package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * The bridge is injected into every frame and cannot be scoped by the platform, so
 * the guard script is what keeps the native API out of article frames. These assert
 * the two halves that make it work: the top document keeps the object, every other
 * frame loses it before its own scripts run.
 */
public class BridgeGuardTest {

    @Test
    public void keepsTheBridgeInTheTopDocument() {
        String script = BridgeGuard.documentStartScript();
        assertTrue(script.contains("if(window.top===window){return;}"));
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
}
