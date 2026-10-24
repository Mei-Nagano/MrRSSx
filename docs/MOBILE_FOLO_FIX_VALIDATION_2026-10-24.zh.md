# 移动端与 Folo 修复验收记录

日期：2026-10-24。基础提交：`101dfa9bffc330acc92d20c59a826909ea19a582`，验收对象为其上的本次工作树修复。按用户要求，Android、布局、Folo 三路均由 **GPT-5.6 Luna / max** 子代理实现，主代理复核、纠正反例并执行集成验证。未提交或推送。

**上次验收的 5 项确定失败均已修复，并通过相应本地回归。17 项当前为 0 项确定失败、12 项部分通过、5 项限定本地范围通过。** 原生 WebView、系统按键/文件选择器/超时、真实 Folo 服务未执行，不能据此宣称完整设备验收通过。

全程未调用 ADB，未连接、操作真机或模拟器，未安装或启动 APK，未使用真实账号。只运行源码检查、本地合成数据测试、headless Chromium、JVM、编译和打包。保留已有验收文档及其他原始改动。

## 1. 五项修复及反例结果

| 项目 | 修复内容 | 本次证据 |
| --- | --- | --- |
| M01 原生桥隔离 | Java 接口校验每个 WebView 的随机能力令牌；仅应用默认端口的 `/`、`/index.html` 主文档获得包装器；同源原文代理页、子 frame、外部来源均排除 | JVM 门禁/入口测试、真实生成的 guard 脚本 Node 执行通过；入口响应头不允许跨源读取；Chromium opaque iframe 读入口被拒，公共资源读取成功 |
| M08 无扩展名模块 MIME | Go 资源代理通过 `meta=1` 返回最终有效 MIME 与 base64 body，Android 解码后优先使用该类型，未知类型对原始 body 回退；保留既有后缀修正 | 本地 HTTP 服务返回 `text/javascript; charset=utf-8` 的无扩展名 module，Go/JVM 链路回归通过；旧 byte-only 响应回退通过 |
| R01 低高度菜单 | 移除 160px 最低高度，按底栏真实顶边限制空间，必要时上翻，保留内部滚动及返回顺序 | 原 639×240 几何反例通过；390×360 滚动、末项可见、菜单先关闭通过；单次浏览器全套 14/14 |
| F01 密集时间旧状态 | 窄窗口未找到目标时最多追加一页 100 条查询；仍无法确认时明确 warning；重查和 live pull 不改变推送轮转时间戳 | 第 11 条无本地 pending 的远端变化能够回传；不可确认目标保留 pending 并报告不完整；501 映射中 pending 不被重查挤出下一批 |
| F04 时间分页 | 精确时间边界与 ID 去重，保留亚毫秒条目；有界越过不可枚举时间带并保持回填推进；修正重复计数 | 单边界、多完整页、回填、500µs、长旧尾均通过；不可枚举时间带明确 Truncated，而非静默成功 |

M01 还有几处与边界直接相关的修正：

- 能力令牌作为独立 Java 参数传递，不混入 Go payload 或原有消息日志；未授权异步调用不向主文档发送 callback。
- 文档开始脚本不受支持时，应用入口 HTML 中的早期包装器和 Java 门禁仍生效。包装器插入 `<head>`，保留 doctype。
- 含令牌的入口 HTML 移除通配 CORS，增加 `CORP: same-origin`、`nosniff`、`no-store`；公共静态资源保留原 CORS 行为。
- 外部顶层导航交给系统浏览器。旧 API 或 redirect 的回退路径先移除接口、停止加载，再在恢复应用的 `loadUrl` 前重新安装接口，遵守“下一次页面加载生效”的平台契约。

定位：[BridgeGuard.java](../build/android/app/src/main/java/com/wails/app/BridgeGuard.java)、[WailsJSBridge.java](../build/android/app/src/main/java/com/wails/app/WailsJSBridge.java)、[WailsPathHandler.java:74](../build/android/app/src/main/java/com/wails/app/WailsPathHandler.java#L74)、[MainActivity.java:396](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L396)。平台依据：[原生桥 frame 边界](https://developer.android.com/privacy-and-security/risks/insecure-webview-native-bridges)、[JavaScript 接口生效时间](https://developer.android.com/reference/android/webkit/WebView#addJavascriptInterface(java.lang.Object,java.lang.String))。

M08 的主路径在 [media_proxy.go:1822](../internal/handlers/media/media_proxy.go#L1822)、[ResourceMetadata.java](../build/android/app/src/main/java/com/wails/app/ResourceMetadata.java)。R01 的定位逻辑提取到 [articleToolbarMenuPosition.ts](../frontend/src/components/article/articleToolbarMenuPosition.ts)。Folo 的查询扩展在 [recheck.go](../internal/folo/recheck.go)，不改轮转的数据库更新在 [folo_sync_db.go:214](../internal/database/folo_sync_db.go#L214)。

## 2. Folo 边界的实际结果

以下均使用本地 HTTP mock 与独立 SQLite 合成数据，不是远端账号烟测。

| 场景 | 本次结果 |
| --- | --- |
| 99 较新 + 2 边界同时间 + 1 更旧 | 102/102 映射，无静默缺口 |
| 97 较新 + 5 边界同时间 + 101 更旧 | 203/203 映射，无静默缺口 |
| 下一条比第一页最旧条目早 500 微秒 | 101/101 映射 |
| 回填窗口可枚举的时间带 | 完整映射并清除已完成回填游标 |
| 回填 101 同时间 + 更旧尾条 | 无法枚举的时间带明确报告截断，较旧条目仍能推进 |
| live 101 同时间 + 600 旧尾，连续两轮 | 时间带 100/101，旧尾 600/600，Truncated 持续为 true；允许的已报告不完整，不记作全量完成 |
| 已映射同时间第 11 条改远端状态 | 通过有界扩展回传；另一条本地 pending 保持优先 |
| 超过一页且目标无法确认 | 保留本地状态/待推送快照，Truncated=true 触发既有 UI warning |
| 501 映射的 pending 轮转 | live/recheck 均不提前 stamp，待推送记录保留在正确处理批次 |

独立复核最初将长尾场景的 700/701 总数视为失败。进一步读取映射确认 600 条旧尾全部完成，缺的仅为不可枚举时间带中的 1 条且已明确标记，故撤回该判断；没有为凑齐 701 条而降低或更改标准。

相应新增回归：[分页](../internal/folo/pagination_regression_test.go)、[重查](../internal/folo/recheck_regression_test.go)、[轮转](../internal/folo/recheck_rotation_regression_test.go)。

## 3. 最终代码的验证与构建

| 检查 | 结果 |
| --- | --- |
| frontend `npm run test:unit` | **73 文件、346 项通过** |
| frontend `npm run type-check` | 通过 |
| frontend `npm run build` | 通过；已有 Vite 配置及大 chunk 警告保留 |
| 目标前端文件 ESLint / Prettier | 子代理执行通过；未运行全库 autofix |
| `go test -count=1 -timeout=5m ./internal/...` | **全部通过，未使用测试缓存** |
| `go vet ./...` | 通过 |
| `go test api_proxy.go api_proxy_test.go` | 通过 |
| JVM 单测 | **18/18**：BridgeGuard 7、ResourceMime 9、WailsPathHandler 2 |
| Java compile | 通过 |
| Chromium 原场景及新增布局/图库场景 | **单次完整运行 14/14 通过**，另先行定向 3/3 |
| Chromium opaque iframe 跨源读取 | **1/1 通过**：入口不可读，公共资源可读的正对照通过 |
| Windows production、server 编译 | 均通过；没有启动生成的 exe |
| Android 双架构 production Go 共享库 + release APK | `package.sh --abi all` 通过；确认包含 arm64-v8a、x86_64 两份 libwails.so |
| `git diff --check` | 通过 |
| Android 完整 lint | **退出 1：2 errors、23 warnings**；两项错误仍为既有 WailsBridge.java:961/976 的 MissingPermission，未绕过或宣称通过 |
| Go race | 未执行，宿主 CGO_ENABLED=0 且未配置宿主 C 编译器；Android NDK 的交叉编译成功不代表宿主 race 可运行 |

Windows/server 对应当前 Taskfile 与 server 说明，前端构建先于 Go 嵌入：

```powershell
$fixOutput = Join-Path $env:TEMP 'mrrss-mobile-fixes-20261024'
New-Item -ItemType Directory -Force -Path $fixOutput | Out-Null
go build -tags production -trimpath -buildvcs=false '-ldflags=-w -s -H windowsgui' -o (Join-Path $fixOutput 'MrRSS.exe') .
go build -tags server -o (Join-Path $fixOutput 'mrrss-server.exe') .
```

Android 使用仓库现有发布构建入口，已检查脚本没有设备操作：

```powershell
$env:JAVA_HOME = 'C:/Program Files/Android/Android Studio/jbr'
$env:ANDROID_HOME = 'C:/Users/ushio/AppData/Local/Android/Sdk'
$env:ANDROID_NDK_HOME = 'C:/Users/ushio/AppData/Local/Android/Sdk/ndk/29.0.14206865'
& 'C:/Program Files/Git/bin/bash.exe' build/android/package.sh --abi all
```

产物：[build/bin/MrRSS.apk](../build/bin/MrRSS.apk)，25,929,063 字节；SHA-256：`feae48b767c3647f21e44c3ae4c5764b95c28c1dea99ca4e1d04f6d1acae70b8`。这是本地 release 配置产物，未发布、安装或运行，也没有声称使用正式发布签名。

## 4. 证据区分与可复现入口

- [完整浏览器运行](evidence/browser-20261024-fixes/result-full.json)：七种宽度 320/390/639/640/767/768/1024、两种低高度场景、卡片/图库返回、筛选保存重开、插件断点。使用新生产前端和 mock API，外部网络拦截。
- [浏览器运行器](evidence/run-acceptance-browser-fixes-20261024.cjs)：复用原验收场景，不修改原通过标准。
- [实际 guard 的 Node 复核](evidence/verify-bridge-guard-fixed.cjs)：执行由当前 Java 生成的 guard.js，验证 5 种非可信上下文确实不保留桥对象，以及两个主入口的 payload/独立令牌参数传递。读取 HEAD 旧 guard 做负对照，旧外部顶层仍保留桥；native 对象使用替身。
- [响应头纯逻辑检查](../build/android/build/tmp/bridge-probe/cors-probe.js)：读取当前 Java 导出的 headers.json；此项本身不等于浏览器 CORS 实测。
- [浏览器 CORS 结果](evidence/browser-20261024-fixes/result-cors.json)：将上述响应头策略装载到本地 HTTP fixture，真实 Chromium 中从 sandbox opaque iframe 读取，配有可读公共资源的正对照。它验证 CORS 策略，仍不是原生 WebView/JNI 验收。
- [M08 旧代码负对照](evidence/mobile-fixes-media-negative-control.txt)：Go overlay 只把 media_proxy.go 换回 HEAD 旧实现，执行同一新增回归。旧实现退出 1，因模块正文不是元数据 JSON而断言失败；最终代码的无缓存全套通过。外层记录器退出 0 代表负对照按预期失败，不是旧实现测试通过。
- [Folo 旧算法负对照](evidence/mobile-fixes-folo-negative-controls.txt)：在临时 overlay 中分别禁用扩展查询、恢复 -1ms 边界；前者实际得到第 11 条仍未读，后者再次得到 101/102 与 100/101 映射，子测试均退出 1。保留当前接口和错误报告字段以便编译，因此这是针对旧算法的负对照，不冒称整个旧提交执行结果。
- [JVM XML](../build/android/app/build/test-results/testDebugUnitTest/)、[lint HTML](../build/android/app/build/reports/lint-results-debug.html)：位于构建目录，后续构建可能覆盖。

本地 `docs/evidence/` 和构建输出仍按现有 `.gitignore` 忽略；本记录及主验收表保存可纳入版本管理的结果、命令和限制。未改忽略规则、依赖、版本或发布配置。

## 5. 原 17 项的状态范围

限定本地范围通过：**M05、F01、F03、F04、F05**。其余 **M01、M02、M03、M04、M06、M07、M08、M09、M10、R01、R02、F02** 为部分通过；具体子范围见[主验收表](MOBILE_FOLO_ACCEPTANCE.zh.md)。本次没有重新把原生运行、真实服务或尚未覆盖的并发场景写成已通过。
