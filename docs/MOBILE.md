# Android build / 安卓版本

MrRSS 的安卓版复用同一套 Go 后端和同一份 Vue 界面：Go 代码编译成 `libwails.so`，由原生 WebView 加载内置的前端资源，数据保存在应用私有目录里的 SQLite。没有独立的第二套实现。

The Android build reuses the same Go backend and the same Vue interface. The Go code compiles to `libwails.so`, a native WebView loads the embedded frontend, and SQLite lives in the app-private directory. There is no second implementation to keep in sync.

## Requirements / 环境要求

- Go (see `go.mod`), Node.js and npm
- Android SDK with platform-tools, an SDK platform and the NDK
- JDK 21 (Android Studio's bundled JBR works)
- `ANDROID_HOME` pointing at the SDK; `ANDROID_NDK_HOME` is optional (the newest installed NDK is used)

以上依赖中，SDK 与 JDK 由 Android Studio 自带即可；`ANDROID_HOME` 必须指向 SDK 目录。

## Build / 构建

日常开发只在模拟器上跑，用这条快速循环：

```sh
task android:dev              # 前端 + Go + 打包 + 装到模拟器 + 启动
task android:dev GO_ONLY=1    # 只改了 Go，跳过前端构建
task android:dev NO_INSTALL=1 # 只打包，不安装
```

等价脚本 `bash build/android/dev.sh [--go-only] [--no-install]`，可用
`ANDROID_SERIAL` 指定目标（默认 `emulator-5554`）。它只编译 x86_64（模拟器
唯一的 ABI）、不 `clean`、并复用 Gradle daemon，所以每次迭代输出各阶段耗时：

```
--- frontend
==> frontend                      17s
==> libwails.so (x86_64, debug)    8s
==> assembleDebug (29M)           19s
==> install + launch              13s
==> running (pid 4948)
```

`--go-only` 大约 40 秒。前端 17 秒和 Gradle 19 秒是当前的两个大头，安装约
13 秒主要是推送 29 MB 的 debug APK。

需要出包时才用下面这些（都是全量，慢）：

```sh
# Debug APK for a connected phone (arm64)
task android:run:device

# Debug APK without installing
task android:build

# Debug APK for the emulator (x86_64)
task android:build ARCH=amd64

# Release APK (signed with the debug keystore unless ANDROID_KEYSTORE_* is set)
task android:package

# Release APK containing both arm64 and x86_64
task android:package:fat
```

产物位于 `build/bin/`。手机开启「开发者选项 → USB 调试」后，用 `adb install -r` 安装。

没装 Task CLI 时，等价的发布构建是一个脚本：

```sh
bash build/android/package.sh                 # dual-ABI release -> build/bin/MrRSS.apk
bash build/android/package.sh --abi arm64     # phone only   -> build/bin/MrRSS-arm64-v8a.apk
bash build/android/package.sh --abi x86_64    # emulator only -> build/bin/MrRSS-x86_64.apk
```

不用本地工具链时，`.github/workflows/android.yml` 会在 GitHub Actions 里构建同样的两个包
（全 ABI 与 arm64），手动触发或在推送 `v*` 标签时运行，产物在 workflow 的 artifact 中下载。

The same workflow (`.github/workflows/android.yml`) builds both packages on GitHub
Actions — manual dispatch or a `v*` tag — and publishes them as workflow artifacts,
so no local Android toolchain is needed.

> `task android:dev` / `dev.sh` 会删掉 `jniLibs/arm64-v8a/libwails.so`，让 APK 只含
> x86_64。之后要出真机包，用 `task android:build ARCH=arm64` 或 `package.sh`
> 重新生成即可。

### Two variants / 两个变体

| File | Variant | Use it for |
| --- | --- | --- |
| `build/bin/MrRSS-debug.apk` | debug, arm64 + x86_64, unminified | Running and debugging on the Pixel 10 emulator (`chrome://inspect`, logcat, stack traces) |
| `build/bin/MrRSS.apk` | release, arm64 + x86_64, R8 + stripped | Sharing one build; works on a phone and the emulator |
| `build/bin/MrRSS-arm64-v8a.apk` | release, arm64 only, R8 + stripped | Installing on a real phone (about 13 MB smaller) |

The debug variant deliberately keeps R8 off: renaming breaks stack traces and
WebView debugging. The release variant is compressed on both sides — R8 with
resource shrinking for the Java/Kotlin code, `-ldflags="-w -s"` plus `-trimpath`
for the Go library (which is the bulk of the size: 46.5 MB → 36.4 MB per ABI).

调试包不启用 R8（混淆会破坏堆栈和 WebView 调试），发布包两头都压：Java 侧 R8 +
资源收缩，Go 侧 `-w -s` 去符号 + `-trimpath`。

The emulator image is `x86_64`; a build that only contains `arm64-v8a` installs
but fails at `System.loadLibrary`. Build with `ARCH=amd64` or use the fat APK.

模拟器镜像是 x86_64，只含 arm64 的包能装但会在 `System.loadLibrary` 处失败。

## Phone interface / 手机界面

The same bundle adapts to the screen it runs on; there is no second UI.

| Width | Layout |
| --- | --- |
| Below 768px | Bottom navigation bar, full-width list, article detail slides over it |
| 768px and wider | Desktop layout: icon rail on the left, list and detail side by side |

On phones the activity bar moves from the left edge to the bottom of the screen
(`ActivityBar.vue`) and carries **only the five views** — All, Unread, Favorites,
Read Later, Gallery. A bottom bar answers "which view am I on"; it is not a
command strip, so the drawer toggle stays out of it. The drawer opens from the
hamburger that leads the list and gallery headers on phones; both sit in the top
left corner at the same size, which is also where the desktop rail keeps the same
control. Add-feed and settings are actions rather than views, so they live in the
drawer footer:

| Action | Where it goes on a phone | Where it stays on desktop |
| --- | --- | --- |
| Add feed | Feed drawer footer | Left rail bottom |
| Settings | Feed drawer footer | Left rail bottom |
| Open feed drawer | Hamburger at the top left of the list and gallery headers | Left rail bottom |

The drawer footer is hidden at ≥768px so the actions always have exactly one
home per layout — the footer is a phone affordance, the rail is the desktop one.
The feed drawer becomes a full-height sheet that starts at the screen edge and
stops above the bar, so its footer stays tappable; it also stacks above the
article pane, otherwise opening it from an article would leave it underneath.
The panes stop above the bar on every build, not only the native shell: the bar
is fixed to the viewport in a browser too. The old 44px rail cost more than a
tenth of a 390px-wide reading area while sitting where a thumb is not.

The gallery header is the one place that does not fit a phone: its full action
row is wider than a 390px screen, which used to push the refresh button past the
right edge and wrap the title onto two lines. On phones it keeps the hamburger,
the title, the sort toggle and an overflow menu; the media filter, the unread and
text toggles, mark-all-read and refresh move into that menu. The row is unchanged
from 768px up.

Touch targets and sheet width follow the same rule: a phone is not a small mouse.
Header icon buttons (hamburger, mark-all-read, unread filter, more, refresh, sort)
get a 40px square hit area instead of the 26px they use on the desktop rail, and
the feed drawer widens to 320px (capped at 84vw) because it is a sheet over the
list rather than a column beside it — 240px clipped longer feed titles. In the
feed management settings the search field takes its own row on phones; sharing
the line with the sort controls cut its placeholder down to "Search fee".

The drawer's own header — pin, sort, close, and the edit toggle beside the search
field — was still 26px, the first row a thumb reaches for; it is 40px on phones
and keeps the compact padding from 640px up. Two smaller ones inside an article
are handled without changing their look: the feed name under the title grows its
hit area with a pseudo-element (16px of text, 40px of target), and the
"Fetch Full Article" button gets 40px and full opacity below the phone
breakpoint, since its 60% opacity is a hover affordance that reads as disabled
when nothing can hover.

A view can also filter the drawer down to nothing — the gallery only lists feeds
with image mode on, unread only lists feeds with unread articles — and an empty
drawer reads as "my feeds vanished". It now says which of the four cases it is:
no feeds at all, no search match, image mode off, or nothing in this view.

Forms had the same problem in a different place. The knob is
`--ui-control-height`: every select, settings input and number field reads it, so
it is 40px below the breakpoint and they all move together (it was 38px).
`.input-field` — the feed form and the rule editor size their inputs with padding
alone and never read the variable — gets the same floor. Modal footers stack full
width on a phone and were still 36px tall; they are 44px. A dialog's own `&times;`
was a 13×32 glyph with no accessible name, and is now a 40px button labelled
`common.close`. The inline "or try a custom script / XPath / email" actions were
as tall as their text (16px) and gain a hit area from vertical padding, which on
an inline box does not move the line they sit on. In the drawer a feed row was
28px — the most tapped row in the app — and is 40px, except in compact mode,
which is a density choice the user made on purpose.

Both image viewers — the one inside an article and the one in the gallery — had
the same two faults. Their toolbars used `top-4` / `bottom-4`, which measure from
the padding box, so the safe-area padding on the root never moved them and the
controls landed under the status bar and the gesture bar. The insets are now
added to the offsets themselves, which changes nothing on the desktop where they
resolve to `0`. Their buttons were also 32px, the one screen where the only input
is a thumb; phones get 40px squares and the dense padding returns from 640px up.

Settings opens as a full-bleed page on phones (no scrim margin, no dialog border,
the panel takes the status-bar inset itself) while staying the centred dialog it
has always been on wider screens. Its shortcut tab is dropped on touch devices:
a phone has no keyboard to bind and no way to press one, so the tab would be a
page of keys nobody can use. The test is `(pointer: coarse)` rather than the
width breakpoint — a phone in landscape is wider than 767px and still has no
keyboard, and a narrow desktop window has one.

Scrollbars drop from 10px to 4px below the phone breakpoint. Styling
`::-webkit-scrollbar` turns Chromium's overlay bars into ones that take layout
space, so the settings tab strip was handing 11px of a 65px row to a bar nobody
drags on a touch screen; the same 10px was eating the list's right edge.

The article table is a desktop layout with a `600px` minimum width, so on a
411px screen it became a sideways-scrolling table with the date column off the
right edge. Below the breakpoint the list renders its normal rows instead —
they already carry the feed and the date under the title — and the layout picker
drops the table option, keeping it only while it is the value already in use so
a phone that inherited it from a synced desktop can still switch away.

System bars are reserved through CSS environment variables. `main.ts` adds a
`mobile-shell` class when it detects the native shell, and the stylesheet uses it
to apply `env(safe-area-inset-*)`. Neither `display-mode: standalone` nor the
phone breakpoint is enough: the WebView reports a browser display mode, and a
landscape phone is wider than the breakpoint while still having a status bar.

Gestures and buttons:

| Interaction | Behaviour |
| --- | --- |
| Hardware back | Closes the topmost layer — menu, viewer, dialog, article, feed drawer — and only exits when nothing is open |
| Swipe right from the left edge | Leaves the article; restricted to the edge so it never fights horizontal scrolling in content |
| Pull down at the top of the list or gallery | Refreshes; the bar grows with the gesture and the list follows it, so it reads as dragging rather than a banner |
| Feed drawer | Full-height sheet; picking a filter from the bottom bar also dismisses it |

The back button is handled in `MainActivity.onBackPressed`, which asks the page
(`window.__mrrssHandleBack`) before falling back to history and then `finish()`.
Components register a handler with `useAndroidBack(handler, priority)`; the
highest priority handler that reports the press as handled wins, and the
priorities in `BACK_PRIORITY` spell out the stack: article `0`, drawer `10`,
app sheets `100`, dialogs `500` (plus their z-index, so stacked dialogs unwind
one at a time), popovers `600`, find-in-page `650`, full-screen viewer `700`.
Popovers that open and close many times without unmounting — the sort menu, the
gallery overflow — use `useDismissOnBack`, which adds and removes the entry with
the open flag instead of with the component's lifetime. Without this a press on a
phone left the app from under an open menu, which was the last thing that made
the build feel unlike a phone app. The context menu's rows also grow to 44px on
phones; 36px is a desktop size.

Toasts are fixed to the viewport rather than to the padded container, so the
native shell offsets them by the status-bar inset as well.

同一个前端按屏宽自适应，没有第二套界面。窄于 768px 时左侧图标栏变成底部导航栏，底栏**只放五个
视图**（全部/未读/收藏/稍后读/图库）——底栏回答"我在看哪一类"，不是命令条，所以抽屉开关不进去。
抽屉从列表与图库顶栏左上角的汉堡打开，两个页面位置和尺寸一致，也和桌面左栏那个按钮同角。添加订阅
与设置属于动作而非视图，放在抽屉页脚（≥768px 时页脚隐藏，桌面在左栏底部）。订阅抽屉是贴边的全高
浮层，底部停在底栏上方，页脚才点得到；它同时叠在文章之上，否则从文章里打开会被压住。内容区在所有
构建里都停在底栏上方，不只是原生壳——浏览器构建的底栏同样固定在视口底部。旧的 44px 左栏会永久
吃掉 390px 阅读区的一成多，位置也够不着拇指。图库顶栏是唯一在手机上放不下的一行：完整操作区比
390px 屏幕还宽，曾把刷新按钮挤出右边缘、标题折成两行；现在手机只留汉堡、标题、排序和溢出菜单，
媒体筛选、只看未读、文字开关、全部已读和刷新都收进菜单，≥768px 保持原样。触控目标和抽屉宽度同理
——手机不是小号鼠标：顶栏的图标按钮（汉堡、全部已读、只看未读、更多、刷新、排序）在手机上给 40px
方形命中区，而不是桌面左栏那套 26px；订阅抽屉在手机上放宽到 320px（上限 84vw），因为它是盖在列表
上的浮层而不是并排的栏，240px 会把较长的订阅名截掉。设置里的订阅管理页，搜索框在手机上独占一行，
和排序控件挤在一行时 placeholder 会被截成「Search fee」。抽屉自己的头部（置顶、排序、关闭，以及搜索框
旁的编辑开关）原来也是 26px，而那是拇指最先够到的一行，手机上一律 40px，640px 起恢复紧凑内边距。文章里
还有两个更小的：标题下的订阅源名用伪元素把命中区撑到 40px（文字仍是 16px），「获取全文」按钮在手机断点
以下给 40px 高和完全不透明——它的 0.6 透明度是悬停提示，没有悬停时就显得像禁用。另外，视图本身可能把
抽屉过滤空（图库只列开启图片模式的订阅源，未读只列有未读的），空抽屉看着像「订阅源丢了」，现在会区分
四种情况：还没订阅源、搜索无匹配、图片模式未开、这个视图下没有内容。表单里也是同一类问题，只是藏在另一处：
总开关是 `--ui-control-height`，所有下拉、设置输入框、数字框都读它，手机断点以下从 38px 抬到 40px，一次全动；
`.input-field`（订阅表单、规则编辑器只靠内边距定高，不读那个变量）补同样的下限。弹窗页脚在手机上会铺满整行、
竖着堆，却仍是 36px 高，改成 44px；对话框自己的 `&times;` 原本是 13×32 的字形、也没有无障碍名称，现在是 40px
的按钮、带 `common.close` 标签。「或者试试 自定义脚本 / XPath 支持 / 邮件订阅」这类行内动作原来只有文字那么高
（16px），靠上下内边距撑出命中区——行内盒子的纵向内边距不影响行高，所以那行字不会被推开。抽屉里的订阅行只有
28px，那是全应用点得最多的一行，现在 40px；紧凑模式除外，那是用户自己选的密度。两个看图器（文章内看图、
图库全屏看图）犯的是同一个错：工具栏用 `top-4` / `bottom-4` 定位，而它量的是 padding box，
根节点的安全区留白根本推不动它，控件就钻到状态栏和手势条底下；现在把 inset 直接加进偏移里，
桌面端 inset 为 `0` 时和原来完全一致。它们的按钮同时是 32px——而这是唯一一个只用拇指操作的界面，
手机上给 40px 方块，640px 起恢复紧凑内边距。手机端设置改为铺满整屏
的页面（无遮罩留边、无对话框描边，面板自己承担状态栏留白），宽屏仍是原来的居中的弹窗。触屏设备上
「快捷键」标签页直接不出现——手机没有可绑定的键盘，也没有按下去的机会，留着只是一页没人能用的按键表。
判断用 `(pointer: coarse)` 而不是宽度断点：横屏手机比 767px 宽但仍然没有键盘，而窄的桌面窗口有。滚动条在
手机断点以下从 10px 收到 4px：一旦自己写 `::-webkit-scrollbar`，Chromium 就不再画悬浮条而是占布局，设置
标签条 65px 的一行里有 11px 给了没人会在触屏上拖的滚动条，列表右侧也被同样的 10px 吃掉。文章表格是
桌面版式，最小宽度 600px，放进 411px 的屏幕就成了横向滚动、日期列跑到右边界外的表格；手机断点以下
列表改回普通行（订阅源和时间本来就在标题下面），布局选择器里也不再提供「表格」——只有当它已经是当前值
时才保留，好让从桌面同步过来的手机能改回别的模式。系统栏留白
由 `main.ts` 添加的 `mobile-shell` 类配合 `env(safe-area-inset-*)` 处理——`display-mode:
standalone` 在原生 WebView 里不成立，而横屏手机的宽度又超过手机断点，只能按平台判断。返回键由
`MainActivity` 先询问页面（`window.__mrrssHandleBack`），页面用 `useAndroidBack`
注册的处理器按优先级依次关闭弹窗、文章、抽屉，都没接管才退出应用；从左边缘右滑也能退出文章，手势限定
在边缘，避免和正文横向滚动冲突。优先级写在 `BACK_PRIORITY` 里，一眼能看出整叠层级：文章 0、抽屉 10、
应用级面板 100、对话框 500（再加各自的 z-index，叠着开的对话框会一层层退）、浮层 600、页内查找 650、
全屏看图 700。会反复开关但不卸载的浮层（排序菜单、图库溢出菜单）用 `useDismissOnBack`，跟着开关状态
登记和撤销，而不是跟着组件生命周期——没有这一层，手机上按返回键会从打开的菜单里直接退出应用，这是最后
一个让原生版「不像手机应用」的地方。右键菜单的行高在手机上同时抬到 44px，36px 是桌面的尺寸。列表
和图库顶部下拉可刷新：指示条随手指长高、内容跟着下移，读起来是「把内容拽下来」而不是「上方弹出横幅」。
Toast 固定在视口上而不是在留白过的容器里，所以原生壳额外按状态栏高度下移，否则会钻到时钟底下。

## What the app does differently / 与桌面版的差异

| Area | Android behaviour |
| --- | --- |
| Startup registration / tray | Not present; the app is a normal launcher activity |
| In-app updates | Hidden; update the APK through the store or by reinstalling |
| Data directory | Fixed to the app sandbox; the picker is hidden |
| OPML import | Works through the system file picker (the file is copied into the app cache) |
| OPML export | Save dialogs are not available on Android; export fails with an error |
| Custom CSS | Import works through the system file picker |
| Opening the original article | Opens the system browser through a VIEW intent |
| Custom script subscriptions | Not supported; the script interpreter is not available |
| Reading mode / full-text extraction | Uses the HTTP fetcher; the headless-Chrome path is unavailable |
| Background refresh | Runs while the app is alive; a foreground service keeps it alive after switching away (see below) |

## Background refresh / 后台刷新

Android may kill a backgrounded process at any time, which would stop the refresh
scheduler. When the app goes to the background, `main-android.go` raises the
foreground service that ships with Wails (`WailsForegroundService`), whose only
job is to keep the process alive with an ongoing notification; it is dropped as
soon as the activity comes back, so the notification does not linger while the
user is in the app.

Three details that are easy to get wrong:

1. **The notification permission is asked while the window is visible.** Android
   drops a permission dialog requested after `onStop`, so `MainActivity.onStart`
   asks once (Android 13+). A refusal is respected: the service then never starts
   and refresh falls back to running only while the app is open.
2. **A foreground service without that permission runs invisibly.** Android
   silently drops its notification, leaving the user with no way to see or stop
   it, so `WailsBridge.startForegroundService` waits for the answer before
   starting rather than starting and hoping.
3. **`refresh_mode = never` gets no service at all.** Keeping a process alive to
   do no work is pure battery cost.

The notification text follows the stored `language` setting, and is re-read each
time the service starts, so changing the language does not require a restart.

安卓系统随时可能回收后台进程，刷新调度器就会随之停止。因此应用退到后台时，
`main-android.go` 会拉起 Wails 自带的前台服务（`WailsForegroundService`）保活，返回前台
立即停止，避免用户正在使用界面时一直挂着通知。

三个容易做错的点：通知权限必须在窗口可见时申请（`onStop` 之后请求的对话框会被系统丢弃），
所以放在 `MainActivity.onStart` 且只问一次，用户拒绝就尊重其选择、后台刷新退化为“仅应用
存活时刷新”；没有通知权限的前台服务会静默运行（系统直接丢掉通知），用户既看不到也无法停止，
所以 `WailsBridge.startForegroundService` 要等权限结果再启动，而不是先启动再说；
`refresh_mode = never` 完全不启动服务——让进程空转只为耗电。通知文案跟随 `language`
设置，且每次启动服务时重新读取，改语言不需要重启。

## How the WebView reaches the backend / 前端如何访问后端

The Android host does not run a local HTTP server. It renders the frontend from
`http://wails.localhost` and answers every request in-process through
`WebViewAssetLoader` → `shouldInterceptRequest` → the Go asset handler. Three
things shape the code:

1. **`/index.html` must not redirect.** `net/http`'s file server answers
   `/index.html` with a 301 to `./`, and the Wails host rewrites the initial
   `/` navigation to `/index.html` while treating any non-200 reply as a missing
   asset. A redirect therefore turns into a blank WebView that tries to resolve
   `wails.localhost` over the network and fails with「无法连接」. `indexRewrite`
   in `main-shared.go` serves the file directly instead.
2. **Request bodies and query strings do not survive that path.** The WebView
   API exposes neither. So `frontend/src/utils/apiBridge.ts` installs a `fetch`
   shim inside the native host: same-origin `/api/` calls are serialised and
   replayed through the Wails runtime transport into `APIProxy` (`api_proxy.go`),
   which executes them against the very same `http.ServeMux` the desktop build
   uses. Every endpoint keeps working unchanged; the shim is a no-op in a
   browser and in the desktop window.
3. **The origin is http, not https.** The template serves the page over https,
   which makes every remote resource in an article mixed content; Chromium
   blocks blockable mixed content even with `MIXED_CONTENT_COMPATIBILITY_MODE`,
   so plain-http feed images and iframes silently disappeared. Serving the page
   over `http://wails.localhost` (`WAILS_SCHEME` plus `setHttpAllowed(true)`)
   removes the mismatch: http subresources match the page scheme and https ones
   are upgrades. Nothing leaves the device — the asset loader answers every
   request to that host in-process. `android:usesCleartextTraffic="true"` in the
   manifest covers the same question for any request that does go out.

安卓端没有本地 HTTP 服务：WebView 从 `http://wails.localhost` 加载界面，请求由
`WebViewAssetLoader` 在进程内转交给 Go。由此带来三条约束——`/index.html` 不能返回
301（否则界面空白并报「无法连接」）；`fetch` 的 body/query 会被 WebView 丢掉，因此
`/api/` 请求改由 `apiBridge.ts` 经 Wails 传输通道转给 `APIProxy` 重放；页面源用 http
而不是模板默认的 https，否则文章里的 http 图片和 iframe 会被 Chromium 当作混合内容拦截
（`MIXED_CONTENT_COMPATIBILITY_MODE` 也救不回来），而页面本身仍由进程内资产处理器应答，
不出设备。

### Subresource requests / 子资源请求

The shim above only covers `fetch`. Requests the browser makes on its own —
`<img>` pointing at the media proxy, an `<iframe>` showing the proxied article —
never reach it, and `WebViewAssetLoader` strips their query string. Since those
routes carry their whole request in the query, the handler saw a bare path,
answered 404, and the WebView retried `wails.localhost` over the network and
reported `ERR_CONNECTION_REFUSED`. Every article image that came from a proxy URL
was affected, along with the original-webpage view.

`MainActivity.shouldInterceptRequest` now gives `/api/` the same treatment as
`/wails/`: pass the path *and* the query to the Go handler. The response is then
mimed by sniffing the bytes (`URLConnection.guessContentTypeFromStream`, falling
back to `text/html` for `/api/webpage/` and `application/json` otherwise), because
an extension-less path has no type to read. A miss returns an explicit 404 rather
than `null` so the failure surfaces instead of degrading into a network error.

One consequence worth knowing: the response is buffered whole, on both the Go side
(`httptest.ResponseRecorder`) and the Java side. That is the existing shape of the
Android transport and is fine for images; it is not a streaming path.

上面的 fetch 垫片只覆盖 `fetch`。浏览器自己发的请求——指向媒体代理的 `<img>`、加载原文
的 `<iframe>`——根本走不到垫片，而 `WebViewAssetLoader` 会剥掉它们的 query。这些路由的
参数全在 query 里，于是处理器只看到光秃秃的路径、返回 404，WebView 转头去真实网络请求
`wails.localhost`，最终报 `ERR_CONNECTION_REFUSED`。凡是走代理 URL 的文章图片和「查看原文」
都受此影响。

现在 `shouldInterceptRequest` 对 `/api/` 采用与 `/wails/` 相同的处理：把路径连同 query
一起交给 Go。响应类型从字节嗅探得出（`URLConnection.guessContentTypeFromStream`，嗅不出
时 `/api/webpage/` 兜底 `text/html`、其余兜底 `application/json`），因为无扩展名的路径没有
类型可读。未命中要显式返回 404 而不是 `null`，否则错误会退化成网络错误。

一个需要知道的代价：响应会在 Go 侧（`httptest.ResponseRecorder`）和 Java 侧各缓冲一整份。
这是安卓传输通道既有的形态，对图片没问题，但不是流式路径。

## Storage / 数据存储

The database is the same SQLite schema everywhere, but not the same Go driver:

| Build | Driver | Why |
| --- | --- | --- |
| Desktop / server / tests | `modernc.org/sqlite` (pure Go) | No C toolchain needed, so `go build` and `go test` work on a bare machine |
| Android | `github.com/mattn/go-sqlite3` (cgo) | Goes through bionic, which uses the `*at()` syscalls Android allows |

The pure-Go driver emits raw `stat`/`lstat`/`unlink` system calls from its
transpiled libc. Android's seccomp policy forbids that whole family for 64-bit
processes — only `newfstatat`, `unlinkat` and friends are permitted — so the
process dies with `SIGSYS` the moment SQLite opens the database. arm64 happens
to have no such syscall numbers, which is why a phone appeared to work while an
x86_64 emulator crashed immediately.

`internal/database/driver_android.go` and `driver_other.go` hold the driver name,
the connection-parameter syntax (`_pragma=name(value)` versus `_busy_timeout=…`)
and the error-code extraction; everything else is shared.

安卓端用的是 cgo 版 SQLite，而不是桌面端的纯 Go 版：纯 Go 驱动会直接发
`stat`/`lstat`/`unlink` 这类老系统调用，而 Android 的 seccomp 策略禁止 64 位进程使用它们，
SQLite 一打开数据库进程就被 `SIGSYS` 杀掉。arm64 恰好没有这些系统调用号，所以真机没事、
x86_64 模拟器立刻崩溃。两个驱动的差异被收敛在 `driver_android.go` / `driver_other.go` 里。

## Where the code lives / 代码位置

| File | Purpose |
| --- | --- |
| `main-android.go` | Android entrypoint: data directory, database, routes, single window |
| `main_android.go` | Registers `main` with the native bridge (`c-shared` builds do not call it) |
| `main-shared.go` | Frontend embedding, the `/index.html` rewrite and the API/asset handler shared by desktop, mobile and server |
| `api_proxy.go` | Replays `/api/` calls coming from the mobile WebView against the API mux |
| `internal/database/driver_android.go` | cgo SQLite driver, DSN syntax and error codes for Android |
| `internal/database/driver_other.go` | Pure-Go SQLite driver for desktop, server and tests |
| `internal/handlers/browser/browser_handlers_android.go` | Opens links through an Android intent |
| `build/android/` | Gradle project, manifest, icons and build tasks |
| `build/android/dev.sh` | Fast x86_64-only debug loop for the emulator (`task android:dev`) |
| `build/android/package.sh` | Release APK into `build/bin/`; `--abi all\|arm64\|x86_64` picks the ABIs (`task android:package:fat`) |
| `frontend/src/utils/platform.ts` | Hides desktop-only settings in the mobile shell |
| `frontend/src/utils/apiBridge.ts` | `fetch` shim that routes `/api/` through the native bridge |
| `frontend/src/composables/ui/useAndroidBack.ts` | Back-handler stack behind the hardware back button |

`main.go` is excluded from Android builds with `//go:build !server && !android && !ios`, so desktop-only code (tray, single instance, window state, the loopback integration API) never compiles into the app.

桌面专属代码（托盘、单实例、窗口状态、本机集成 API）通过构建标签排除，不会编译进安卓包。

## Application ID / 应用包名

The store-facing ID is `com.mrrss.app` (`applicationId` in `build/android/app/build.gradle`). The Java classes stay in `com.wails.app` because the native bridge exports fixed JNI symbols (`Java_com_wails_app_WailsBridge_*`). Renaming those classes requires mirroring the exports on the Go side.

对外包名是 `com.mrrss.app`；Java 类仍在 `com.wails.app` 下，因为原生桥接的 JNI 符号是固定的。

## Status / 当前状态

The Android target builds, installs and runs with a phone-shaped interface:
bottom navigation, safe-area handling, the hardware back button, swipe-back,
background refresh that survives switching away, and proxied article images and
the original-webpage view through a query-preserving subresource handler. What is
still missing is surviving the task being swiped out of recents — the process is
gone at that point, and only a scheduled job (WorkManager or `AlarmManager`) could
wake it up again; that is not implemented.

安卓版本可以构建、安装并运行，界面已经是手机形态：底部导航、系统栏留白、返回键、右滑返回、
切到后台仍继续的后台刷新，以及经保留 query 的子资源处理器正常显示的文章图片与「查看原文」。
仍缺的只有“从最近任务里划掉后还能定时唤醒”——此时进程已经没了，只有 WorkManager 或
AlarmManager 这类调度任务能重新拉起，这一块尚未实现。
