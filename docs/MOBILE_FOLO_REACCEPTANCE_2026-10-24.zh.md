# 移动端、响应式布局与 Folo 第二次修复验收

验收日期：2026-10-24。基线：`101dfa9bffc330acc92d20c59a826909ea19a582`；主要修复提交：`bf123267e3fb410aa118f044fa077c0735550f77`。开始时工作树干净。

**整体验收未通过：17 项中 5 项失败、9 项部分通过、3 项在限定本地范围内通过。** 上轮失败中的图库返回、POST body 取消、Folo 推送轮转已有新通过证据；原生桥、资源 MIME、Folo 旧状态重查及分页仍有失败，另发现低高度视口的菜单与底栏重叠。

本轮遵守“不动真机”的要求：未调用 ADB，未连接、操作真机或模拟器，未安装或启动 APK，未使用真实 Folo 账号。应用代码没有修改，只更新验收文档与本地证据。旧设备截图不作为本次通过依据。

## 1. 逐项结论

“部分通过”只确认已执行子范围，不能作为该项完整标准已满足；“限定本地范围通过”也不代表真实服务或设备验收通过。原始标准见[验收用例](MOBILE_FOLO_ACCEPTANCE.zh.md)。

| 项目 | 本轮状态 | 证据和未覆盖范围 |
| --- | --- | --- |
| M01 原文与原生桥隔离 | **失败** | 不支持 document-start 时仍注入桥；外部顶层导航被 guard 放行。源码、平台契约与真实 guard 脚本语义确认，未做设备攻击探针 |
| M02 旧 Android 启动 | 部分通过 | API 分支、Java 编译和 lint 可核对；API 21/23/24 冷启动与生命周期未执行 |
| M03 文章返回保留范围 | 部分通过 | 手机收藏范围返回和 dispatcher 通过；其他范围组合及 Android 系统事件链未完整执行 |
| M04 卡片及图库返回层级 | 部分通过 | 卡片 JS 返回通过；图库媒体先关闭、保留图库，Escape 通过；原生系统返回链未执行 |
| M05 抽屉编辑弹窗 | 限定本地范围通过 | 实际浏览器中编辑、保存至 mock、重开值一致；返回关闭弹窗后抽屉保留 |
| M06 请求取消 | 部分通过 | 旧 POST body 读取中取消现为 AbortError，无 native Do；本地 Go proxy 测试通过；JNI 与真实网络取消链未执行 |
| M07 查询串保真 | 部分通过 | 编码路径和源码保留；WebView/JNI 完整传递未执行 |
| M08 资源 MIME | **失败** | 无扩展名 CSS 已修复；无扩展名合法 module 仍被推断为 text/html |
| M09 Android 选择文件 | 部分通过 | 实现和 Java 编译可核对；系统选择器、取消和异常文件操作未执行 |
| M10 后台超时退出 | 部分通过 | timeout 分支和 Java 编译可核对；Android 15 系统超时未执行 |
| R01 工具栏溢出菜单 | **失败** | 390×360 滚动、末项可见、返回先关菜单通过；639×240 菜单与底栏重叠 30px |
| R02 手机底部导航 | 部分通过 | 七种宽度验证手机清空详情、桌面保留详情；手机表格回退普通行通过；全导航组合、连续旋转和安全区未执行 |
| F01 旧文章远端状态 | **失败** | 双方发布时间差和整 8 条游标回绕已修复；11 条同时间映射中第 11 条连续四轮仍漏查 |
| F02 登录取消后旧回调 | 部分通过 | 本地 HTTP mock 取消和替换门禁通过；完整退出、过期的并发 HTTP 场景未补齐，race 未执行 |
| F03 超过 500 条映射推进 | 限定本地范围通过 | 501 条中最旧和最后一条分别 pending，三轮完整 Sync 内均推送；现有失败重试通过 |
| F04 同发布时间分页 | **失败** | 纯大时间段能报告截断，UI 能显示 warning；单条边界、多完整页、回填和亚毫秒间隔仍漏项且未标截断 |
| F05 迟到账户查询 | 限定本地范围通过 | 10 项组件测试和四种迟到响应顺序通过；未使用真实服务 |

## 2. 仍需修复的五项

### M01 / P1：原生桥的信任边界没有封闭

`addJavascriptInterface` 仍对 WebView 注入桥。`BridgeGuard.install()` 在 DOCUMENT_START_SCRIPT 不可用或安装异常时仅记录日志，不能阻止外部 frame 使用已注入的桥。当前声明支持旧版 Android，不能假定所有 WebView 都提供这个特性。

即使 document-start 可用，guard 的 `if(window.top===window){return;}` 也会放行所有顶层文档，没有校验应用来源。现有视频链接通过 `window.open(..., '_blank')` 打开；WebView 未启用多窗口，也没有顶层导航白名单，外部 host 的请求交回默认处理。这使外部顶层文档也处于放行分支。

实际执行当前 guard 脚本：应用顶层保留桥、外部 iframe 删除桥、外部顶层仍保留桥。这个结果证明脚本语义；导航路径来自源码和 Android 平台契约，不能冒充设备实测。

- 定位：[BridgeGuard.java:37](../build/android/app/src/main/java/com/wails/app/BridgeGuard.java#L37)、[BridgeGuard.java:49](../build/android/app/src/main/java/com/wails/app/BridgeGuard.java#L49)、[MainActivity.java:245](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L245)、[VideoPlayer.vue:58](../frontend/src/components/article/parts/VideoPlayer.vue#L58)。
- 下一次通过条件：桥只对可信应用主文档可用；不支持 guard 和外部顶层导航时也必须保持隔离，并保留主界面正常 API 通信。
- 平台依据：[JavaScript 接口会注入所有 frame](https://developer.android.com/privacy-and-security/risks/insecure-webview-native-bridges)、[WebSettings 多窗口默认行为](https://developer.android.com/reference/android/webkit/WebSettings#setSupportMultipleWindows(boolean))。

### F01 / P1：密集同时间条目的旧状态仍不回传

重查使用远端自己的发布时间及游标回绕的修复有效。但每个映射的时间窗口只取前 10 条，没有按目标 entry ID 继续查找或报告未找到。

本地 mock 中建立 11 条同一发布时间的已映射文章，将远端改为已读；live 游标已推进到更晚时间。连续四轮 Sync 后第 11 条仍未读。若排序稳定，再多轮重查也不能越过固定前 10 条；这是当前实现的推断，四轮未回传是实测事实。

- 定位：[sync.go:43](../internal/folo/sync.go#L43)、[sync.go:613](../internal/folo/sync.go#L613)、[sync.go:621](../internal/folo/sync.go#L621)。
- 下一次通过条件：同时间密集条目中每个已有映射的远端变更都能被有界重查；本地 pending 仍优先。无法查明时需明确报告不完整，不能静默略过。

### F04 / P2：分页恢复仍有静默缺口

新逻辑仅在当前整页最旧时间出现多次时记录边界，且主要在尾页才补查。首个边界仅出现一次、后续完整页覆盖先前边界、回填窗口或小于 1ms 的间隔仍可遗漏。

下列合成条目都运行实际 Sync，并以本地映射数复核前三项影响：

| 场景 | 实测结果 |
| --- | --- |
| 99 条较新 + 2 条边界同时间 + 1 条更旧 | 101/102 被映射，Truncated=false |
| 97 条较新 + 5 条边界同时间 + 101 条更旧 | 201/203 被映射，Truncated=false |
| 回填窗口内 101 条同时间 + 1 条更旧 | 101/102 被映射，Truncated=false |
| 下一条比第一页最旧条目早 500 微秒 | 100/101 被枚举，Truncated=false |

101 条纯同时间、以及带更旧尾条的原反例，现在至少能明确标记截断；前端也追加含时间的 warning。它们不能抵消上表静默遗漏。另有恢复条目被重复计入 Skipped 的统计问题，详见本地 Folo 证据；本报告不为该问题增加第 18 项。

- 定位：[sync.go:303](../internal/folo/sync.go#L303)、[sync.go:332](../internal/folo/sync.go#L332)、[sync.go:344](../internal/folo/sync.go#L344)、[sync.go:369](../internal/folo/sync.go#L369)。
- 下一次通过条件：单个边界、跨页和回填均不静默丢条目；API 无法枚举时明确报告不完整，游标不能掩盖缺口。

### M08 / P2：无扩展名 module 仍使用 HTML 类型

CSS 的上游 Content-Type 传递和 `ct` 修复通过。对于 `<script type="module" src="https://example.test/assets/module">`，资源重写没有提供相同类型信息；实际合法正文 `export const answer = 42;` 在当前 JVM MIME 推断路径中得到空类型，生产分支随后回退 `text/html`。模块脚本要求 JavaScript MIME，因此这一资源类型仍不满足原标准。

- 定位：[MainActivity.java:495](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L495)、[MainActivity.java:517](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L517)、[media_proxy.go:809](../internal/handlers/media/media_proxy.go#L809)、[media_proxy.go:1056](../internal/handlers/media/media_proxy.go#L1056)。
- 下一次通过条件：扩展名不能作为唯一依据，合法脚本等资源的真实类型能穿过代理并由 WebView 正确使用。现有 CSS 通过不能代表其他资源类型通过。

### R01 / P2：低高度视口的菜单覆盖底部导航

新构建前端在 Chromium 中以 **639×240 CSS px** 执行，展开更多菜单后测得菜单底边 214px、底栏顶边 184px，重叠 **30px**。原因是最大高度计算仍使用最少 160px；该视口实际只剩 130px 可用。

同一几何断言在两次执行中都失败。390×360 的内部滚动、末项可见和返回先关菜单仍通过；不能因此将更低高度的失败归为设备限制。

- 定位：[ArticleToolbar.vue:124](../frontend/src/components/article/ArticleToolbar.vue#L124)。
- 下一次通过条件：菜单按实际可用空间收缩或换位，在低高度和安全区条件下不与底栏重叠，同时保留内部滚动和返回顺序。

## 3. 本次执行的检查

| 命令或检查 | 真实结果与限制 |
| --- | --- |
| frontend：`npm run test:unit` | **73 文件、344 项通过**；jsdom，不证明实际排版 |
| frontend：`npm run type-check` | 退出 0 |
| frontend：`npm run build` | 退出 0；新生成生产前端。有既有 Vite 配置及大 chunk 警告 |
| `go test -timeout=5m ./internal/...` | 退出 0；部分包使用缓存 |
| `go test -count=1 -timeout=5m ./internal/folo ./internal/handlers/folo` | 退出 0，定向无缓存复测 |
| `go test -count=1 -timeout=5m ./internal/database -run 'Folo'` | 退出 0 |
| `go test api_proxy.go api_proxy_test.go` | 退出 0 |
| `go vet ./...` | 退出 0 |
| Windows production 与 server 编译 | 均退出 0；输出在 TEMP，未启动程序 |
| Android Java 实际重新编译 | `--rerun-tasks` 退出 0，21 个任务实际执行 |
| Android JVM 单测 | BridgeGuard 4 项、ResourceMime 6 项，**10/10 通过** |
| Android 完整 lint | **退出 1：2 errors、24 warnings，NewApi=0**；两项既有 MissingPermission 位于 WailsBridge.java:961/976，警告增量未作旧 HEAD 对照 |
| Folo 补充 Go overlay | 12 项中 7 通过、5 失败；另 4 项实际映射/统计复核确认失败，不记作通过 |
| 前端补充实脚本 | POST 取消、图库 dispatcher、truncated 反馈、插件断点渲染通过；native Call/网络使用替身 |
| Chromium 主浏览器场景 | 11/11 通过：七宽度、低高度菜单、卡片、表格与范围返回、筛选编辑保存重开 |
| Chromium 新增场景复测 | 2 通过、1 失败：插件断点、图库媒体返回/Escape 通过，639×240 菜单失败 |
| Go race | 未执行：CGO_ENABLED=0，PATH 未发现 gcc/clang；未为此安装工具 |

Android 的具体命令和报告见同轮[Android 本地记录](evidence/acceptance-2026-10-24-recheck2-android.md)。两项 lint 错误在前一轮及旧基线已经存在；未绕过它们，也不将完整 lint 记为通过。

编译前先生成前端，命令与当前 Windows Taskfile、server 说明相符；没有运行会改写依赖和生成文件的完整 Task 链：

```powershell
$acceptanceOutput = Join-Path $env:TEMP 'mrrss-acceptance-20261024-recheck2'
New-Item -ItemType Directory -Force -Path $acceptanceOutput | Out-Null
go build -tags production -trimpath -buildvcs=false '-ldflags=-w -s -H windowsgui' -o (Join-Path $acceptanceOutput 'MrRSS.exe') .
go build -tags server -o (Join-Path $acceptanceOutput 'mrrss-server.exe') .
```

这两项仅为宿主 Windows 上的编译检查，不代表 APK 已重建、Go 共享库已重新嵌入或发布流水线已验收。CI、README 和发布资产不是本轮原 17 项的验收对象。

## 4. 浏览器布局与插件范围

新生产前端在 `127.0.0.1:5187` 由 Vite preview 提供，Cypress 16.1.1、headless Electron/Chromium 146 执行；全部 API 使用合成 fixture，外部网络被拦截。没有启动真实后端或访问用户数据库。

- 320、390、639、640、767、768、1024px：列表、详情、底栏/侧栏断点及 Folo 设置均通过容器边界和横向溢出检查。覆盖中英文、浅深主题的组合，没有声称全组合覆盖。
- 767→768→390px 同次会话验证：窄屏保留 FreshRSS、Miniflux、Folo、RSSHub；宽屏恢复 Obsidian、Notion、SiYuan、Zotero。当前规则按布局宽度，未承诺原生 Android 宽屏仍隐藏四个插件。
- 图库实际卡片打开媒体后，JS 返回和 Escape 都关闭媒体并保留图库选中状态；卡片阅读弹窗返回也通过。
- 人工查看本轮 320/640px Folo 设置、390px RSS 插件页和图库返回后截图；未见横向越界，640px 表单说明较窄、换行密集作为样式残余保留。
- 390×360 菜单行为断言通过。截图操作可能触发视口滚动使菜单关闭，因此没有将菜单截图当作视觉通过依据；639×240 失败来自截图前的实际 DOM 几何测量。

首轮浏览器共 14 个场景：原 11 个通过，新增 3 个失败。新增中两项是验收脚本错误：SiYuan 图标使用 SVG 而非 img，图库图片不接受 pointer events，应点击卡片容器。更正选择器后仅重跑这 3 项，得到 2 通过、1 产品失败；未 force click、未修改产品。按唯一场景计 **13 通过、1 失败**，来自两次执行，不能宣称一次完整运行全绿。

## 5. 本地证据与交付边界

- [Android 复验记录](evidence/acceptance-2026-10-24-recheck2-android.md)
- [前端复验记录](evidence/acceptance-2026-10-24-recheck2-frontend.md)
- [Folo 复验记录](evidence/acceptance-2026-10-24-recheck2-folo.md)
- [浏览器首次结果](evidence/browser-20261024-recheck2/result-full.json)、[新增场景复测](evidence/browser-20261024-recheck2/result-added.json)
- [浏览器执行入口](evidence/run-acceptance-browser-recheck2-20261024.cjs)、[新增场景](evidence/acceptance-browser-recheck2-20261024.cy.js)

`docs/evidence/` 按仓库现有 `.gitignore` 保留为本地材料；本报告及主验收文档保存可纳入版本管理的结论、定位、复现条件和命令，没有修改忽略规则。临时 Go overlay、Node 探针和构建输出未改写应用源码。

本次完成的是无设备验收与文档交付。上述五项失败使候选修复不能整体通过；其他未执行的系统行为继续明确记为部分通过，不使用历史设备记录替代本轮证据。
