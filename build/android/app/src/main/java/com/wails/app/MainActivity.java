package com.wails.app;

import android.annotation.SuppressLint;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.res.Configuration;
import android.database.Cursor;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.NetworkInfo;
import android.net.NetworkRequest;
import android.net.Uri;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Bundle;
import android.os.PowerManager;
import android.content.pm.PackageManager;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.util.Base64;
import android.util.Log;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import androidx.annotation.Nullable;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.content.ContextCompat;
import androidx.core.content.FileProvider;
import androidx.webkit.WebViewAssetLoader;

import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.List;

/**
 * MainActivity hosts the WebView and manages the Wails application lifecycle.
 * It uses WebViewAssetLoader to serve assets from the Go library without
 * requiring a network server.
 */
public class MainActivity extends AppCompatActivity {
    private static final String TAG = "WailsActivity";
    private static final boolean DEBUG = BuildConfig.DEBUG;
    // The page is served from http rather than the template's https. Every
    // request is still answered in-process by the asset loader, so nothing
    // leaves the device; the scheme only decides how the WebView classifies
    // remote subresources. Feeds embed plain-http images and iframes, and a
    // Chromium WebView refuses to load blockable mixed content from an https
    // origin, so an https page would silently drop them. Keeping the origin on
    // http means https resources are upgrades (always allowed) and http
    // resources are same-scheme.
    private static final String WAILS_SCHEME = "http";
    private static final String WAILS_HOST = "wails.localhost";
    private static final int FILE_PICKER_REQUEST = 7001;

    private WebView webView;
    private WailsBridge bridge;
    // Battery: system-event receivers are registered only while the activity is
    // in the foreground (onStart) and torn down in onStop, so background battery/
    // network/screen broadcasts don't wake the app.
    private boolean systemReceiversRegistered = false;
    // The notification permission is asked at most once per process; a refusal
    // means background refresh runs without a visible notification, and asking
    // again on every foreground would just be noise.
    private boolean notificationPermissionAsked = false;
    private WebViewAssetLoader assetLoader;

    // The Go-side dialog ID of the in-flight file picker (-1 when idle)
    private int pendingFilePickerCallbackID = -1;
    private static final int PHOTO_CAPTURE_REQUEST = 7002;
    private static final int VIDEO_CAPTURE_REQUEST = 7003;
    private static final int CAMERA_PERMISSION_REQUEST = 7010;
    // A page-level <input type="file">: the WebView has no picker of its own, so
    // the host must show the system one and hand the result back.
    private static final int WEB_FILE_CHOOSER_REQUEST = 7011;
    private ValueCallback<Uri[]> webFileChooserCallback;
    private File pendingCaptureFile;
    private boolean pendingCaptureIsVideo;

    // System-event sources (battery/power, screen lock, network). Registered in
    // onCreate, torn down in onDestroy. Each forwards a "system:*" event to JS
    // via the bridge.
    private BroadcastReceiver batteryReceiver;
    private BroadcastReceiver screenReceiver;
    private BroadcastReceiver powerSaveReceiver;
    private ConnectivityManager connectivityManager;
    private ConnectivityManager.NetworkCallback networkCallback;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);

        // Initialize the native Go library
        bridge = new WailsBridge(this);
        bridge.initialize();

        // Set up WebView
        setupWebView();

        // Load the application
        loadApplication();
    }

    @SuppressLint("SetJavaScriptEnabled")
    private void setupWebView() {
        webView = findViewById(R.id.webview);
        bridge.setWebView(webView);

        // Configure WebView settings
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMediaPlaybackRequiresUserGesture(false);
        // Belt and braces for the scheme choice above: if the origin ever moves
        // back to https, allow images and iframes from plain-http feeds instead
        // of the WebView's block-everything default.
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE);

        // Enable debugging in debug builds
        if (DEBUG) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        // Set up asset loader for serving local assets. Http has to be allowed
        // explicitly; the builder only accepts https by default.
        assetLoader = new WebViewAssetLoader.Builder()
                .setDomain(WAILS_HOST)
                .setHttpAllowed(true)
                .addPathHandler("/", new WailsPathHandler(bridge))
                .build();

        // Set up WebView client to intercept requests
        webView.setWebViewClient(new WebViewClient() {
            @Nullable
            @Override
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                // Handle wails.localhost requests
                if (request.getUrl().getHost() != null &&
                        request.getUrl().getHost().equals(WAILS_HOST)) {

                    // For wails API calls (runtime, capabilities, etc.) pass the
                    // full URL including the query string, because
                    // WebViewAssetLoader.PathHandler strips query params
                    String path = request.getUrl().getPath();
                    if (path != null && path.startsWith("/wails/")) {
                        String fullPath = path;
                        String query = request.getUrl().getEncodedQuery();
                        if (query != null && !query.isEmpty()) {
                            fullPath = path + "?" + query;
                        }
                        if (DEBUG) Log.d(TAG, "Wails API call: " + fullPath);

                        byte[] data = bridge.serveAsset(fullPath, request.getMethod(), "{}");
                        if (data != null && data.length > 0) {
                            java.io.InputStream inputStream = new java.io.ByteArrayInputStream(data);
                            java.util.Map<String, String> headers = new java.util.HashMap<>();
                            // No Access-Control-Allow-Origin: this is the runtime/native
                            // API and only the app's own same-origin frame consumes it. A
                            // wildcard here would let any framed document, including an
                            // untrusted original page, reach native methods.
                            headers.put("Cache-Control", "no-cache");
                            headers.put("Content-Type", "application/json");

                            return new WebResourceResponse(
                                "application/json",
                                "UTF-8",
                                200,
                                "OK",
                                headers,
                                inputStream
                            );
                        }
                        // Return error response if data is null
                        return new WebResourceResponse(
                            "application/json",
                            "UTF-8",
                            500,
                            "Internal Error",
                            new java.util.HashMap<>(),
                            new java.io.ByteArrayInputStream("{}".getBytes())
                        );
                    }

                    // Stream captured photos/videos from the cache with HTTP Range
                    // support so <video> can seek/stream a clip of any length.
                    if (path != null && path.startsWith("/__capture__/")) {
                        return serveCaptureFile(path.substring("/__capture__/".length()), request);
                    }

                    // API routes reached as subresources — <img> pointing at the
                    // media proxy, an <iframe> showing the proxied article, a
                    // download link — go through the same path surgery as /wails/.
                    // WebViewAssetLoader strips the query string, and for these
                    // routes the query *is* the request: without it the Go handler
                    // sees a bare path and answers 404, which the WebView then
                    // retries over the network against wails.localhost and reports
                    // as ERR_CONNECTION_REFUSED.
                    if (path != null && path.startsWith("/api/")) {
                        String fullPath = path;
                        String query = request.getUrl().getEncodedQuery();
                        if (query != null && !query.isEmpty()) {
                            fullPath = path + "?" + query;
                        }
                        return serveApiSubresource(fullPath, request);
                    }

                    // For regular assets, use the asset loader
                    return assetLoader.shouldInterceptRequest(request.getUrl());
                }

                return super.shouldInterceptRequest(view, request);
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                super.onPageFinished(view, url);
                if (DEBUG) Log.d(TAG, "Page loaded: " + url);
                bridge.onPageFinished(url);
                // Now that JS listeners are mounted, push a snapshot of the
                // current battery / network / theme so the UI starts populated.
                emitSystemSnapshot();
            }
        });

        // Add JavaScript interface for Go communication
        webView.addJavascriptInterface(new WailsJSBridge(bridge, webView), "wails");

        // The interface is injected into every frame, so the frames that are not the
        // app document drop it before their own scripts run.
        BridgeGuard.install(webView);

        // A page-level <input type="file"> has no picker inside the WebView; without
        // this callback the tap does nothing at all (rule import, backup restore).
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> filePathCallback,
                                             FileChooserParams params) {
                // Release a stale callback first, otherwise the page's next file
                // input never gets an answer.
                if (webFileChooserCallback != null) {
                    webFileChooserCallback.onReceiveValue(null);
                }
                webFileChooserCallback = filePathCallback;
                try {
                    Intent intent = params.createIntent();
                    intent.addCategory(Intent.CATEGORY_OPENABLE);
                    startActivityForResult(intent, WEB_FILE_CHOOSER_REQUEST);
                    return true;
                } catch (Exception e) {
                    Log.e(TAG, "file chooser failed", e);
                    webFileChooserCallback = null;
                    filePathCallback.onReceiveValue(null);
                    return false;
                }
            }
        });
    }

    private void loadApplication() {
        String url = WAILS_SCHEME + "://" + WAILS_HOST + "/";
        if (DEBUG) Log.d(TAG, "Loading URL: " + url);
        webView.loadUrl(url);
    }

    /**
     * Launch the system camera to capture a photo (video=false) or a video
     * (video=true). The capture is written to a FileProvider URI in the cache and
     * the result is delivered to JS as a "common:capture" event.
     */
    public void launchCameraCapture(boolean video) {
        if (ContextCompat.checkSelfPermission(this, "android.permission.CAMERA")
                != PackageManager.PERMISSION_GRANTED) {
            // checkSelfPermission/requestPermissions are API 23. Below that a manifest
            // permission is granted at install time, so there is nothing to request and
            // calling either method would throw NoSuchMethodError.
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                pendingCaptureIsVideo = video;
                requestPermissions(new String[]{"android.permission.CAMERA"}, CAMERA_PERMISSION_REQUEST);
                return;
            }
        }
        try {
            File dir = new File(getCacheDir(), "captures");
            if (!dir.exists()) dir.mkdirs();
            pendingCaptureFile = new File(dir, "capture_" + System.currentTimeMillis() + (video ? ".mp4" : ".jpg"));
            pendingCaptureIsVideo = video;
            Uri uri = FileProvider.getUriForFile(this, getPackageName() + ".fileprovider", pendingCaptureFile);
            Intent intent = new Intent(video ? MediaStore.ACTION_VIDEO_CAPTURE : MediaStore.ACTION_IMAGE_CAPTURE);
            intent.putExtra(MediaStore.EXTRA_OUTPUT, uri);
            intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
            // Don't pre-check with resolveActivity(): Android 11+ package visibility
            // hides other apps' intents unless declared in <queries>, so it can
            // return null even when a camera app exists. Just launch and handle a miss.
            startActivityForResult(intent, video ? VIDEO_CAPTURE_REQUEST : PHOTO_CAPTURE_REQUEST);
        } catch (android.content.ActivityNotFoundException e) {
            bridge.emitEvent("common:capture", "{\"error\":\"no camera app available\"}");
        } catch (Exception e) {
            Log.e(TAG, "launchCameraCapture failed", e);
            bridge.emitEvent("common:capture", "{\"error\":\"capture failed\"}");
        }
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == CAMERA_PERMISSION_REQUEST) {
            if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                launchCameraCapture(pendingCaptureIsVideo);
            } else {
                bridge.emitEvent("common:capture", "{\"error\":\"camera permission denied\"}");
            }
            return;
        }
        if (requestCode == WailsBridge.NOTIFICATION_PERMISSION_REQUEST) {
            // The answer decides whether the background refresh service can run
            // with a visible notification; the bridge reports a refusal.
            if (bridge != null) {
                bridge.resumeForegroundServiceIfPermitted();
            }
            return;
        }
        if (bridge != null) {
            bridge.onRequestPermissionsResult(requestCode, grantResults);
        }
    }

    private void handleCaptureResult(int resultCode, @Nullable Intent data) {
        File file = pendingCaptureFile;
        final boolean video = pendingCaptureIsVideo;
        pendingCaptureFile = null;
        if (resultCode != RESULT_OK) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        // Some camera apps (commonly for video) ignore EXTRA_OUTPUT and instead
        // return a content URI in the result data; copy that into our cache.
        if ((file == null || !file.exists() || file.length() == 0)
                && data != null && data.getData() != null) {
            String copied = copyUriToCache(data.getData());
            if (copied != null) file = new File(copied);
        }
        final File f = file;
        if (f == null || !f.exists() || f.length() == 0) {
            bridge.emitEvent("common:capture", "{\"cancelled\":true}");
            return;
        }
        new Thread(() -> {
            try {
                JSONObject o = new JSONObject();
                o.put("type", video ? "video" : "photo");
                o.put("path", f.getAbsolutePath());
                o.put("size", f.length());
                if (!video) {
                    String thumb = makePhotoThumbnail(f);
                    if (thumb != null) o.put("thumb", thumb);
                }
                // Stream URL works for both: <video>/<img> load it from the cache
                // via shouldInterceptRequest (Range-enabled), no size limit.
                o.put("streamUrl", captureStreamUrl(f));
                bridge.emitEvent("common:capture", o.toString());
            } catch (Exception e) {
                Log.e(TAG, "handleCaptureResult failed", e);
                bridge.emitEvent("common:capture", "{\"error\":\"result processing failed\"}");
            }
        }).start();
    }

    /** Downscale a captured photo into a base64 JPEG data URL for display in the webview. */
    @Nullable
    private String makePhotoThumbnail(File file) {
        try {
            BitmapFactory.Options bounds = new BitmapFactory.Options();
            bounds.inJustDecodeBounds = true;
            BitmapFactory.decodeFile(file.getAbsolutePath(), bounds);
            int sample = 1;
            while (Math.max(bounds.outWidth, bounds.outHeight) / sample > 640) sample *= 2;
            BitmapFactory.Options opts = new BitmapFactory.Options();
            opts.inSampleSize = sample;
            Bitmap bmp = BitmapFactory.decodeFile(file.getAbsolutePath(), opts);
            if (bmp == null) return null;
            ByteArrayOutputStream baos = new ByteArrayOutputStream();
            bmp.compress(Bitmap.CompressFormat.JPEG, 70, baos);
            bmp.recycle();
            return "data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Build a same-origin URL the webview can stream a capture from. Served by
     * serveCaptureFile (via shouldInterceptRequest); the path is relative to the
     * cache dir so both camera files (captures/) and copied content URIs
     * (wails-picker/) resolve.
     */
    private String captureStreamUrl(File file) {
        String base = getCacheDir().getAbsolutePath() + File.separator;
        String abs = file.getAbsolutePath();
        String rel = abs.startsWith(base) ? abs.substring(base.length()) : file.getName();
        return "/__capture__/" + Uri.encode(rel, "/");
    }

    /**
     * Answer an /api/ request that arrived as a subresource rather than through
     * the fetch shim. The Go handler needs the query string, and the response has
     * no file extension to read a MIME type from, so the type is sniffed from the
     * bytes. A failed request returns 404 instead of null: returning null lets the
     * WebView fall back to a real network request for wails.localhost, which
     * surfaces as ERR_CONNECTION_REFUSED instead of the actual error.
     *
     * This channel carries only the method and path — nativeServeAsset ignores its
     * third argument and serveAssetForAndroid takes no body — so it is read-only by
     * construction. That is fine: GET/HEAD are the only methods a browser issues
     * for img/iframe/resource subresources, and every request with a body (every
     * write) reaches Go through the fetch shim instead, never here.
     */
    @Nullable
    private WebResourceResponse serveApiSubresource(String fullPath, WebResourceRequest request) {
        byte[] data;
        try {
            // The "{}" argument is a placeholder nativeServeAsset does not read;
            // see the method note above.
            data = bridge.serveAsset(fullPath, request.getMethod(), "{}");
        } catch (Exception e) {
            Log.e(TAG, "serveApiSubresource failed for " + fullPath, e);
            data = null;
        }

        if (DEBUG) Log.d(TAG, "API subresource: " + fullPath + " (" + (data == null ? 0 : data.length) + " bytes)");

        if (data == null || data.length == 0) {
            return new WebResourceResponse(
                    "text/plain", "UTF-8", 404, "Not Found",
                    new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
        }

        String mimeType = sniffMimeType(data, fullPath);
        java.util.Map<String, String> headers = new java.util.HashMap<>();
        // The original-page iframe is sandboxed without allow-same-origin, so it runs
        // in an opaque origin and needs CORS to read its own proxied assets (fonts and
        // module scripts require it; images and stylesheets do not). Scoped to the
        // proxy routes deliberately: a wildcard on every /api/ route would let such a
        // frame read article and settings data.
        if (fullPath.startsWith("/api/webpage/")) {
            headers.put("Access-Control-Allow-Origin", "*");
        }
        headers.put("Cache-Control", "no-cache");

        // Charset only makes sense for text; declaring it on image/png makes some
        // WebView versions re-encode the payload.
        String encoding = mimeType.startsWith("text/") || mimeType.endsWith("json")
                || mimeType.endsWith("javascript") || mimeType.endsWith("xml") ? "UTF-8" : null;

        return new WebResourceResponse(mimeType, encoding, 200, "OK", headers,
                new java.io.ByteArrayInputStream(data));
    }

    /**
     * Guess a MIME type from the payload. {@code URLConnection.guessContentType-
     * FromStream} covers the common feed assets (HTML, PNG, JPEG, GIF, PDF).
     *
     * Stylesheets, module scripts and fonts carry no magic bytes, and answering
     * text/html for them makes the WebView refuse to apply them, so the original
     * URL — still present as url_b64 — supplies the extension instead. Only when
     * neither signal exists does the path-based fallback apply.
     */
    /** Reads a percent-encoded query parameter; the base64 ones go through queryParam. */
    @Nullable
    private String plainQueryParam(String fullPath, String name) {
        try {
            return android.net.Uri.parse("http://localhost" + fullPath).getQueryParameter(name);
        } catch (RuntimeException e) {
            return null;
        }
    }
    private String sniffMimeType(byte[] data, String path) {
        String hint = ResourceMime.fromTypeHint(plainQueryParam(path, "ct"));
        if (hint != null) {
            return hint;
        }
        try (java.io.ByteArrayInputStream in = new java.io.ByteArrayInputStream(data)) {
            String sniffed = java.net.URLConnection.guessContentTypeFromStream(in);
            if (sniffed != null) {
                return sniffed;
            }
        } catch (Exception ignored) {
            // Fall through to the extension and path guesses.
        }

        String upstream = queryParam(path, "url_b64");
        if (upstream != null) {
            String byExtension = mimeFromFileName(upstream);
            if (byExtension != null) {
                return byExtension;
            }
        }

        if (path.startsWith("/api/webpage/")) {
            return "text/html";
        }
        return "application/json";
    }

    /**
     * Reads and base64-decodes a query parameter. The proxies build these URLs by
     * concatenating standard base64, where '+' reaches the parser as a space, so
     * it is restored before decoding.
     */
    @Nullable
    private String queryParam(String fullPath, String name) {
        try {
            String raw = android.net.Uri.parse("http://localhost" + fullPath).getQueryParameter(name);
            if (raw == null || raw.isEmpty()) {
                return null;
            }
            byte[] decoded = android.util.Base64.decode(
                    raw.replace(' ', '+'), android.util.Base64.DEFAULT);
            return new String(decoded, java.nio.charset.StandardCharsets.UTF_8);
        } catch (Exception e) {
            return null;
        }
    }

    /** Maps a resource URL's extension to the type the WebView expects. */
    @Nullable
    private String mimeFromFileName(String url) {
        return ResourceMime.fromUrl(url);
    }

    /**
     * Serve a captured file (under the app cache) to the webview with HTTP Range
     * support, so &lt;video&gt; can stream and seek a clip of any length without
     * inlining it as a data URL.
     */
    private WebResourceResponse serveCaptureFile(String relPath, WebResourceRequest request) {
        try {
            File cache = getCacheDir();
            File file = new File(cache, Uri.decode(relPath));
            // Path-traversal guard: only ever serve files under the cache dir.
            if (!file.getCanonicalPath().startsWith(cache.getCanonicalPath() + File.separator)
                    || !file.exists() || !file.isFile()) {
                return new WebResourceResponse("text/plain", "UTF-8", 404, "Not Found",
                        new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
            }
            String name = file.getName().toLowerCase();
            String mime = name.endsWith(".mp4") ? "video/mp4"
                    : name.endsWith(".mov") ? "video/quicktime"
                    : name.endsWith(".jpg") || name.endsWith(".jpeg") ? "image/jpeg"
                    : name.endsWith(".png") ? "image/png" : "application/octet-stream";
            long length = file.length();
            java.util.Map<String, String> reqHeaders = request.getRequestHeaders();
            String range = reqHeaders != null ? reqHeaders.get("Range") : null;
            if (range == null && reqHeaders != null) range = reqHeaders.get("range");

            java.util.Map<String, String> headers = new java.util.HashMap<>();
            headers.put("Accept-Ranges", "bytes");
            headers.put("Cache-Control", "no-store");

            if (range != null && range.startsWith("bytes=")) {
                long start = 0, end = length - 1;
                String spec = range.substring(6).trim();
                int dash = spec.indexOf('-');
                if (dash >= 0) {
                    try {
                        if (dash > 0) start = Long.parseLong(spec.substring(0, dash).trim());
                        String e = spec.substring(dash + 1).trim();
                        if (!e.isEmpty()) end = Long.parseLong(e);
                    } catch (NumberFormatException ignored) { }
                }
                if (start < 0) start = 0;
                if (end >= length) end = length - 1;
                if (start > end) { start = 0; end = length - 1; }
                long count = end - start + 1;
                java.io.InputStream in = new java.io.FileInputStream(file);
                long toSkip = start;
                while (toSkip > 0) {
                    long s = in.skip(toSkip);
                    if (s <= 0) break;
                    toSkip -= s;
                }
                headers.put("Content-Range", "bytes " + start + "-" + end + "/" + length);
                headers.put("Content-Length", String.valueOf(count));
                return new WebResourceResponse(mime, null, 206, "Partial Content",
                        headers, new LimitedInputStream(in, count));
            }
            headers.put("Content-Length", String.valueOf(length));
            return new WebResourceResponse(mime, null, 200, "OK", headers,
                    new java.io.FileInputStream(file));
        } catch (Exception e) {
            Log.e(TAG, "serveCaptureFile failed", e);
            return new WebResourceResponse("text/plain", "UTF-8", 500, "Error",
                    new java.util.HashMap<>(), new java.io.ByteArrayInputStream(new byte[0]));
        }
    }

    /** Wraps a stream to yield at most a fixed number of bytes (for Range responses). */
    private static final class LimitedInputStream extends java.io.FilterInputStream {
        private long remaining;
        LimitedInputStream(java.io.InputStream in, long limit) {
            super(in);
            this.remaining = limit;
        }
        @Override public int read() throws java.io.IOException {
            if (remaining <= 0) return -1;
            int b = super.read();
            if (b >= 0) remaining--;
            return b;
        }
        @Override public int read(byte[] b, int off, int len) throws java.io.IOException {
            if (remaining <= 0) return -1;
            int n = super.read(b, off, (int) Math.min(len, remaining));
            if (n > 0) remaining -= n;
            return n;
        }
    }

    /**
     * Launch the system document picker. Results are copied into the app's
     * cache directory so Go receives real filesystem paths. Called by
     * WailsBridge on the main thread.
     */
    public void launchFilePicker(int callbackID, boolean multiple) {
        synchronized (this) {
            if (pendingFilePickerCallbackID != -1) {
                // Only one picker can be in flight
                bridge.filePickerDone(callbackID);
                return;
            }
            pendingFilePickerCallbackID = callbackID;
        }

        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple);
        try {
            startActivityForResult(intent, FILE_PICKER_REQUEST);
        } catch (Exception e) {
            Log.e(TAG, "Failed to launch file picker", e);
            pendingFilePickerCallbackID = -1;
            bridge.filePickerDone(callbackID);
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, @Nullable Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == PHOTO_CAPTURE_REQUEST || requestCode == VIDEO_CAPTURE_REQUEST) {
            handleCaptureResult(resultCode, data);
            return;
        }
        if (requestCode == WEB_FILE_CHOOSER_REQUEST) {
            // A cancelled picker must still answer, or the page's <input> stays busy.
            if (webFileChooserCallback != null) {
                Uri[] result = WebChromeClient.FileChooserParams.parseResult(resultCode, data);
                webFileChooserCallback.onReceiveValue(result);
                webFileChooserCallback = null;
            }
            return;
        }
        if (requestCode != FILE_PICKER_REQUEST) {
            return;
        }
        final int callbackID = pendingFilePickerCallbackID;
        pendingFilePickerCallbackID = -1;
        if (callbackID == -1) {
            return;
        }

        final List<Uri> uris = new ArrayList<>();
        if (resultCode == RESULT_OK && data != null) {
            if (data.getClipData() != null) {
                for (int i = 0; i < data.getClipData().getItemCount(); i++) {
                    uris.add(data.getClipData().getItemAt(i).getUri());
                }
            } else if (data.getData() != null) {
                uris.add(data.getData());
            }
        }

        // Copy the documents off the main thread, then notify Go
        new Thread(() -> {
            for (Uri uri : uris) {
                String path = copyUriToCache(uri);
                if (path != null) {
                    bridge.filePickerResult(callbackID, path);
                }
            }
            bridge.filePickerDone(callbackID);
        }).start();
    }

    /**
     * Copy a content URI into the app cache and return its filesystem path.
     */
    @Nullable
    private String copyUriToCache(Uri uri) {
        String name = "document";
        try (Cursor cursor = getContentResolver().query(uri, null, null, null, null)) {
            if (cursor != null && cursor.moveToFirst()) {
                int idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (idx >= 0 && cursor.getString(idx) != null) {
                    name = new File(cursor.getString(idx)).getName();
                }
            }
        } catch (Exception ignored) {
        }

        try {
            File dir = new File(getCacheDir(), "wails-picker/" + System.nanoTime());
            if (!dir.mkdirs()) {
                return null;
            }
            File out = new File(dir, name);
            try (InputStream in = getContentResolver().openInputStream(uri);
                 OutputStream os = new FileOutputStream(out)) {
                if (in == null) {
                    return null;
                }
                byte[] buf = new byte[64 * 1024];
                int n;
                while ((n = in.read(buf)) > 0) {
                    os.write(buf, 0, n);
                }
            }
            return out.getAbsolutePath();
        } catch (Exception e) {
            Log.e(TAG, "Failed to copy picked document", e);
            return null;
        }
    }

    /**
     * Execute JavaScript in the WebView from the Go side
     */
    public void executeJavaScript(final String js) {
        runOnUiThread(() -> {
            if (webView != null) {
                webView.evaluateJavascript(js, null);
            }
        });
    }

    // ---- System events ---------------------------------------------------
    // Battery/power, screen lock and network connectivity are surfaced to JS as
    // "system:*" events. The OS broadcasts used here (ACTION_BATTERY_CHANGED,
    // SCREEN_OFF, USER_PRESENT, POWER_SAVE_MODE_CHANGED) are protected system
    // broadcasts, so dynamic registration needs no RECEIVER_* export flag.

    private void registerSystemEventReceivers() {
        // Battery + charging state (sticky broadcast: the current value is
        // delivered to the receiver immediately on registration).
        batteryReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(intent);
            }
        };
        registerReceiver(batteryReceiver, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));

        // Low-power (battery saver) mode toggles → re-emit battery with the flag.
        powerSaveReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
            }
        };
        registerReceiver(powerSaveReceiver,
                new IntentFilter(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED));

        // Screen lock / unlock. SCREEN_OFF ≈ locked; USER_PRESENT = unlocked.
        screenReceiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                String action = intent.getAction();
                if (Intent.ACTION_SCREEN_OFF.equals(action)) {
                    emitLock(true);
                } else if (Intent.ACTION_USER_PRESENT.equals(action)) {
                    emitLock(false);
                }
            }
        };
        IntentFilter screenFilter = new IntentFilter();
        screenFilter.addAction(Intent.ACTION_SCREEN_OFF);
        screenFilter.addAction(Intent.ACTION_USER_PRESENT);
        registerReceiver(screenReceiver, screenFilter);

        // Network connectivity / transport type / cellular signal strength.
        connectivityManager = (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
        if (connectivityManager != null) {
            networkCallback = new ConnectivityManager.NetworkCallback() {
                @Override public void onAvailable(Network network) { emitNetwork(network); }
                @Override public void onLost(Network network) { emitNetworkDisconnected(); }
                @Override public void onCapabilitiesChanged(Network network, NetworkCapabilities caps) {
                    emitNetwork(network);
                }
            };
            try {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
                    connectivityManager.registerDefaultNetworkCallback(networkCallback);
                } else {
                    // registerDefaultNetworkCallback arrived in API 24, but minSdk is 21.
                    // On API 21-23 the call throws NoSuchMethodError, which is an Error
                    // rather than an Exception, so the catch below never intercepts it
                    // and the app dies during startup. Registering against an explicit
                    // request is the API 21 equivalent for the same callback.
                    NetworkRequest request = new NetworkRequest.Builder()
                            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                            .build();
                    connectivityManager.registerNetworkCallback(request, networkCallback);
                }
            } catch (Exception e) {
                Log.e(TAG, "network callback registration failed", e);
            }
        }
    }

    private void unregisterSystemEventReceivers() {
        safeUnregister(batteryReceiver);
        batteryReceiver = null;
        safeUnregister(powerSaveReceiver);
        powerSaveReceiver = null;
        safeUnregister(screenReceiver);
        screenReceiver = null;
        if (connectivityManager != null && networkCallback != null) {
            try {
                connectivityManager.unregisterNetworkCallback(networkCallback);
            } catch (Exception ignored) {
            }
            networkCallback = null;
        }
    }

    private void safeUnregister(BroadcastReceiver r) {
        if (r != null) {
            try {
                unregisterReceiver(r);
            } catch (Exception ignored) {
            }
        }
    }

    /** Read the current sticky value for an action without a standing receiver. */
    @Nullable
    private Intent registerSticky(String action) {
        return registerReceiver(null, new IntentFilter(action));
    }

    /** Push current battery / network / theme so a freshly-loaded UI is populated. */
    private void emitSystemSnapshot() {
        emitBattery(registerSticky(Intent.ACTION_BATTERY_CHANGED));
        if (connectivityManager != null) {
            // getActiveNetwork arrived in API 23; older devices read the legacy
            // NetworkInfo instead of crashing on a missing method.
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                Network active = connectivityManager.getActiveNetwork();
                if (active != null) {
                    emitNetwork(active);
                } else {
                    emitNetworkDisconnected();
                }
            } else {
                emitLegacyNetwork();
            }
        }
        emitTheme();
    }

    /**
     * Connectivity snapshot for API 21-22. Emits the same payload shape as
     * {@link #emitNetwork} so the frontend keeps a single code path.
     */
    @SuppressWarnings("deprecation")
    private void emitLegacyNetwork() {
        try {
            NetworkInfo info = connectivityManager != null
                    ? connectivityManager.getActiveNetworkInfo() : null;
            boolean connected = info != null && info.isConnected();
            String type = "none";
            if (info != null) {
                if (info.getType() == ConnectivityManager.TYPE_WIFI) {
                    type = "wifi";
                } else if (info.getType() == ConnectivityManager.TYPE_MOBILE) {
                    type = "cellular";
                } else if (info.getType() == ConnectivityManager.TYPE_ETHERNET) {
                    type = "wired";
                } else {
                    type = connected ? "other" : "none";
                }
            }
            JSONObject o = new JSONObject();
            o.put("connected", connected);
            o.put("type", type);
            o.put("metered", connectivityManager.isActiveNetworkMetered());
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitLegacyNetwork failed", e);
        }
    }

    private void emitBattery(@Nullable Intent batteryStatus) {
        try {
            float level = -1f;
            String state = "unknown";
            if (batteryStatus != null) {
                int lvl = batteryStatus.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
                int scale = batteryStatus.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
                if (lvl >= 0 && scale > 0) {
                    level = lvl / (float) scale;
                }
                switch (batteryStatus.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
                    case BatteryManager.BATTERY_STATUS_CHARGING: state = "charging"; break;
                    case BatteryManager.BATTERY_STATUS_FULL: state = "full"; break;
                    case BatteryManager.BATTERY_STATUS_DISCHARGING:
                    case BatteryManager.BATTERY_STATUS_NOT_CHARGING: state = "unplugged"; break;
                    default: state = "unknown"; break;
                }
            }
            boolean lowPower = false;
            PowerManager pm = (PowerManager) getSystemService(Context.POWER_SERVICE);
            if (pm != null) {
                lowPower = pm.isPowerSaveMode();
            }
            JSONObject o = new JSONObject();
            o.put("level", (double) level);
            o.put("state", state);
            o.put("lowPowerMode", lowPower);
            if (bridge != null) bridge.emitSystemEvent("android:BatteryChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitBattery failed", e);
        }
    }

    private void emitNetwork(@Nullable Network network) {
        try {
            boolean connected = false;
            String type = "none";
            boolean metered = false;
            Integer signal = null;
            if (connectivityManager != null && network != null) {
                NetworkCapabilities caps = connectivityManager.getNetworkCapabilities(network);
                if (caps != null) {
                    connected = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET);
                    if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) {
                        type = "wifi";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) {
                        type = "cellular";
                    } else if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) {
                        type = "wired";
                    } else {
                        type = "other";
                    }
                    metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        int s = caps.getSignalStrength();
                        if (s != Integer.MIN_VALUE) {
                            signal = s; // dBm; closer to 0 is a stronger signal
                        }
                    }
                }
            }
            JSONObject o = new JSONObject();
            o.put("connected", connected);
            o.put("type", type);
            o.put("metered", metered);
            if (signal != null) {
                o.put("signal", (int) signal);
            }
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception e) {
            Log.e(TAG, "emitNetwork failed", e);
        }
    }

    private void emitNetworkDisconnected() {
        try {
            JSONObject o = new JSONObject();
            o.put("connected", false);
            o.put("type", "none");
            o.put("metered", false);
            if (bridge != null) bridge.emitSystemEvent("android:NetworkChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    private void emitLock(boolean locked) {
        // Lock/unlock are signals (no payload); name carries the state.
        if (bridge != null) {
            bridge.emitSystemEvent(locked ? "android:ScreenLocked" : "android:ScreenUnlocked", "{}");
        }
    }

    private void emitTheme() {
        try {
            int mode = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
            JSONObject o = new JSONObject();
            // "isDarkMode" matches the context key the desktop platforms use.
            o.put("isDarkMode", mode == Configuration.UI_MODE_NIGHT_YES);
            if (bridge != null) bridge.emitSystemEvent("android:ThemeChanged", o.toString());
        } catch (Exception ignored) {
        }
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        // Fires for light/dark switches because the manifest lists uiMode in
        // android:configChanges (otherwise the activity would be recreated).
        emitTheme();
    }

    @Override
    protected void onStart() {
        super.onStart();
        // Battery: only monitor system events while the app is visible.
        if (!systemReceiversRegistered) {
            registerSystemEventReceivers();
            systemReceiversRegistered = true;
        }
        if (bridge != null) {
            bridge.onStart();
        }
        // Ask for the notification permission while the window is visible: the
        // background refresh service needs it to show its ongoing notification,
        // and a dialog requested after onStop would be dropped by the platform.
        // Asked once; a refusal is respected and never repeated.
        if (Build.VERSION.SDK_INT >= 33
                && !notificationPermissionAsked
                && checkSelfPermission("android.permission.POST_NOTIFICATIONS")
                        != PackageManager.PERMISSION_GRANTED) {
            notificationPermissionAsked = true;
            requestPermissions(
                    new String[]{"android.permission.POST_NOTIFICATIONS"},
                    WailsBridge.NOTIFICATION_PERMISSION_REQUEST);
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (bridge != null) {
            bridge.onResume();
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (bridge != null) {
            bridge.onPause();
        }
    }

    @Override
    protected void onStop() {
        super.onStop();
        if (systemReceiversRegistered) {
            unregisterSystemEventReceivers();
            systemReceiversRegistered = false;
        }
        if (bridge != null) {
            bridge.onStop();
        }
    }

    @Override
    public void onLowMemory() {
        super.onLowMemory();
        if (bridge != null) {
            bridge.onLowMemory();
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        unregisterSystemEventReceivers();
        if (bridge != null) {
            bridge.shutdown();
        }
        if (webView != null) {
            webView.destroy();
        }
    }

    @Override
    public void onBackPressed() {
        if (webView == null) {
            super.onBackPressed();
            return;
        }
        // The page owns the navigation state (open article, feed drawer,
        // settings sheet), so ask it first. A press it does not consume falls
        // through to the shell, which then behaves like any other app.
        webView.evaluateJavascript(
                "(window.__mrrssHandleBack && window.__mrrssHandleBack()) ? '1' : '0'",
                value -> {
                    if (value != null && value.contains("1")) {
                        return;
                    }
                    runOnUiThread(() -> {
                        if (webView != null && webView.canGoBack()) {
                            webView.goBack();
                        } else {
                            finish();
                        }
                    });
                });
    }
}
