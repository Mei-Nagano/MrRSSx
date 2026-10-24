# 移动端、响应式布局与 Folo 插件验收

- 编写日期：2026-10-24。
- 审查基线：`9afd4efa`；功能提交为 `52ec2204`（Folo）和 `fdf861a5`（Android、响应式布局等）。
- 范围：两轮审查确认的 17 项问题，以及修复后需要执行的定向回归。
- 最新复验基线：`101dfa9b` 上的本次工作树修复；用户指定三个 GPT-5.6 Luna 子代理，思考等级均为 `max`。
- 最新结论：2026-10-24 修复复验，**原 5 项确定失败的反例已解除；17 项当前为 0 项确定失败、12 项部分通过、5 项限定本地范围通过**。设备级未执行范围仍不计为通过，详见 [修复验收记录](MOBILE_FOLO_FIX_VALIDATION_2026-10-24.zh.md)。
- 本轮边界：不调用 ADB，不连接或操作真机、模拟器，不使用真实 Folo 账号；已修复应用并执行本地测试、编译和 APK 打包，未安装或启动 APK。
- 本文件记录本次验收范围，不作为项目永久开发规范。

第 4–6 节保留原始审查问题及通过标准；后续旧轮次为历史记录。当前结果以修复验收记录和下表为准。[第二次验收](MOBILE_FOLO_REACCEPTANCE_2026-10-24.zh.md)的 5 项失败及更早的 7 项失败代表各自基线，不覆盖本次结果。无确定失败不等于原生 WebView、系统行为或真实服务已经完成验收。

## 1. 状态与证据口径

初始审查时各项为“待修复、待复验”。本轮按实际证据更新为通过、部分通过或失败；部分通过只确认已执行子范围，不代表该项所有标准已满足，未执行不计为通过。

现有测试通过只能证明其覆盖的行为。临时探针复现缺陷不等于修复后的回归通过，也不等于已经完成设备或真实服务验证。

| 证据标记 | 已执行内容 | 使用限制 |
| --- | --- | --- |
| S | 源码、调用链及必要的平台文档核对 | 不声称已在设备或真实浏览器复现 |
| J | Node/jsdom 中运行实际组件逻辑，必要依赖使用替身 | 不证明 CSS 实际排版、触摸命中区域或 WebView 行为 |
| N | Node 中加载原始 API 桥代码，以延迟原生调用替身验证 | 不证明 JNI 或上游服务已收到取消 |
| G | 本地 HTTP mock 与临时 Go overlay 反例验证 | 不证明真实 Folo 服务的行为 |
| B | 新构建前端在 headless Chromium 中运行，API 全部使用隔离合成数据 | 不证明原生 WebView、真机触摸、系统安全区或真实服务行为 |

临时探针未纳入仓库。修复时应将适用的反例转成可重复运行的回归测试，并在执行记录中填写测试名称、命令和结果。

## 2. 验收环境与数据

以下为待执行的环境覆盖，不代表这些环境已经测试：

| 维度 | 覆盖要求 |
| --- | --- |
| Android 兼容性 | 以当前 `minSdk`、`targetSdk` 为准；基线为 API 21/35，覆盖 API 21、23、24 边界及 API 35 后台服务限制 |
| 浏览器 | 桌面浏览器、手机浏览器；至少覆盖项目实际支持的一种 Chromium 浏览器 |
| 宽度 | 320、390、639、640、767、768、1024 CSS px，覆盖现有断点两侧 |
| 高度与方向 | 低高度窗口、手机横竖屏、窗口缩放；菜单内容超过可用高度 |
| 显示与布局 | 浅色/深色，中英文，列表/表格/卡片/图库布局 |
| Folo | 本地可控 mock 优先；真实服务烟测使用专用测试账号与测试订阅 |
| 数据保护 | 使用独立测试数据目录和合成令牌；验证已有订阅、文章及状态没有被意外删除或改写 |

构建入口见 [移动端说明](MOBILE.md)、[Taskfile](../Taskfile.yml) 和 [Android Taskfile](../build/android/Taskfile.yml)；Folo 功能约定见 [阅读器集成说明](READER_INTEGRATIONS.md)。

## 3. 问题总表

P1 表示需要优先处理的安全、启动或核心功能问题；P2 表示其余已确认功能缺陷。优先级不代替逐项验收结果。

| 编号 | 优先级 | 验收项 | 审查证据 | 修复验收状态 |
| --- | --- | --- | --- | --- |
| M01 | P1 | 外部原文与原生桥接隔离 | S、JVM、脚本语义、B | 部分通过：Java 能力门禁与可信入口限制通过；原生 WebView 未执行 |
| M02 | P1 | 声明支持的旧版 Android 可启动 | S、JVM/lint | 部分通过，旧 API 环境未执行 |
| M03 | P2 | 文章返回保持原列表范围 | S、J、B | 部分通过，原生按键链未执行 |
| M04 | P2 | 卡片阅读弹窗响应系统返回 | S、J、B | 部分通过，卡片/图库 JS 返回通过；原生链未执行 |
| M05 | P2 | 抽屉内弹窗不被外部点击处理销毁 | S、J、B | 通过（本轮隔离浏览器范围） |
| M06 | P2 | Android API 请求取消有效 | S、N、Go | 部分通过，旧 POST 反例修复；JNI 取消链未执行 |
| M07 | P2 | 资源请求查询串保真 | S | 部分通过，WebView/JNI 未执行 |
| M08 | P2 | 原文资源 MIME 类型正确 | S、Go、JVM、打包 | 部分通过：模块 MIME 元数据链通过；WebView 实际加载未执行 |
| M09 | P2 | Android 规则导入可选择文件 | S、编译 | 部分通过，系统选择器未执行 |
| M10 | P2 | 后台服务超时可正常退出 | S、编译 | 部分通过，系统超时未执行 |
| R01 | P2 | 工具栏溢出菜单可滚动 | S、J、B | 部分通过：低高度重叠已修复；设备触摸与安全区未执行 |
| R02 | P2 | 手机底部导航返回目标列表 | S、J、B | 部分通过，部分导航/连续旋转未执行 |
| F01 | P1 | 旧文章的远端状态能够回传 | S、G | 通过（本地 mock：第 11 条扩展重查、未确认警告及 pending 轮转） |
| F02 | P1 | 取消登录后旧回调不能恢复凭据 | S、G | 部分通过，完整退出/过期并发未覆盖 |
| F03 | P2 | 超过 500 条映射时旧修改可推进 | S、G | 通过（本轮本地 HTTP mock/SQLite 范围） |
| F04 | P2 | 同发布时间分页不漏条目 | S、G、J | 通过（本地 mock：边界、多页、回填、亚毫秒；不可枚举时明确警告） |
| F05 | P2 | 迟到的账户查询不覆盖新状态 | S、J | 通过（本地组件/脚本范围） |

## 4. Android 与移动端验收用例

### M01 — 外部原文与原生桥接隔离

- 定位：[MainActivity.java:233](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L233)、[ArticleDetail.vue:192](../frontend/src/components/article/ArticleDetail.vue#L192)。
- 当前问题：原文页面保留脚本，并获得 `window.wails` 桥接；同源原文 iframe 还可能访问主文档。
- 前置条件：准备受控原文页面和只读或内存级测试探针，不使用真实凭据或破坏性接口。
- 操作：在普通阅读页、卡片阅读弹窗打开该原文，尝试调用原生桥、访问主文档及内部 API；同时执行主界面的正常数据操作。
- 通过标准：外部原文脚本不能取得主界面权限、原生能力或内部 API 数据；主界面合法通信仍正常。覆盖直接 iframe、嵌套 iframe 和页面导航路径。
- 当前证据：S；未执行 Android 设备上的安全回归。

### M02 — 声明支持的旧版 Android 可启动

- 定位：[MainActivity.java:702](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L702)、[app/build.gradle:15](../build/android/app/build.gradle#L15)。
- 当前问题：最低版本声明为 API 21，却无版本分支地调用 API 24 才提供的网络回调 API。
- 操作：在声明支持的 API 21、23 设备或模拟器冷启动，切到后台再返回；在 API 24 以上执行相同操作并切换网络。
- 通过标准：支持范围内启动和返回前台均无 `NoSuchMethodError`；网络状态按对应平台能力处理，不因缺少新 API 崩溃。
- 当前证据：S；设备验收未执行。若另行调整最低支持版本，须明确记录范围变更，不能把未测旧版本记为通过。

### M03 — 文章返回保持原列表范围

- 定位：[App.vue:188](../frontend/src/App.vue#L188)、[useAndroidBack.ts:28](../frontend/src/composables/ui/useAndroidBack.ts#L28)。
- 当前问题：列表范围的返回处理优先于文章详情，先执行 `setFilter('all')`。
- 操作：分别从订阅、分类、未读和收藏列表打开文章，按一次 Android 系统返回；再在列表页按返回。
- 通过标准：第一次关闭文章并保留原列表范围；列表页的后续返回按既定导航规则处理。图片查看器、查找面板等更上层界面应先关闭。
- 当前证据：S；系统返回的设备整合验收未执行。

### M04 — 卡片阅读弹窗响应系统返回

- 定位：[ArticleDetailModal.vue:223](../frontend/src/components/article/ArticleDetailModal.vue#L223)、[ArticleList.vue:1885](../frontend/src/components/article/ArticleList.vue#L1885)。
- 当前问题：卡片阅读弹窗只处理键盘 Escape，没有注册 Android 系统返回。
- 操作：卡片布局下，从“全部文章”打开文章弹窗，按系统返回；再覆盖弹窗内查找、图片查看器和图库媒体弹窗。
- 通过标准：逐层关闭最上方界面，回到原列表；文章弹窗打开时不会直接退出应用或切换后台列表范围。
- 当前证据：卡片阅读路径为 S；图库作为同类路径复验，不声称已做设备验证。

### M05 — 抽屉内弹窗不被外部点击处理销毁

- 定位：[Sidebar.vue:82](../frontend/src/components/sidebar/Sidebar.vue#L82)、[FeedList.vue:955](../frontend/src/components/sidebar/FeedList.vue#L955)。
- 当前问题：Teleport 到 `body` 的筛选弹窗被判为抽屉外部，点击后抽屉和弹窗一起卸载。
- 操作：手机宽度下打开订阅抽屉，编辑已保存筛选；点击名称、条件和保存按钮；再验证弹窗外点击及返回关闭行为。
- 通过标准：弹窗内部操作不会销毁其所属界面；保存能完成且重新打开后内容正确；外部点击仍按约定关闭对应层。
- 当前证据：S、J；真实 Sidebar 配合 Teleport 子组件替身，已观察到内部 `pointerdown` 关闭并卸载弹窗。未执行完整界面 E2E。

### M06 — Android API 请求取消有效

- 定位：[apiBridge.ts:85](../frontend/src/utils/apiBridge.ts#L85)、[api_proxy.go:89](../api_proxy.go#L89)。
- 当前问题：桥接没有消费 `AbortSignal`，Go 侧创建的 HTTP 请求也没有继承调用取消。
- 操作：分别使用预先取消的信号、请求中取消的信号；阻塞三个旧文章翻译后切换文章；生成阅读报告后取消。
- 通过标准：预先取消不发起原生请求；处理中取消能结束前端等待并传递到 Go 请求上下文；失效翻译释放队列位置，新文章可继续翻译；取消后不再启动后续上游调用或重试。
- 说明：不以“上游已经开始处理的请求必然免计费”作为验收标准。
- 当前证据：S、N；预先取消、处理中取消仍调用后端并返回成功的反例已复现。真实 JNI 和上游取消未验证。

### M07 — 资源请求查询串保真

- 定位：[MainActivity.java:207](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L207)；同时检查第 157 行的运行时请求分支。
- 当前问题：`getQuery()` 解码后重新拼接，内层 URL 的分隔符可能被 Go 当成外层参数解析。
- 操作：原文地址使用 `https://example.test/post?a=1&token=a%2Bb`，经正常原文入口加载；增加空格、中文和百分号编码用例，以 mock 记录实际收到的目标 URL。
- 通过标准：目标 URL、查询参数值和编码语义完整保留；原文、媒体及运行时请求均不发生额外解码或参数截断。
- 当前证据：S 和查询解码示例；未进行 WebView 端到端请求验证。

### M08 — 原文资源 MIME 类型正确

- 定位：[MainActivity.java:408](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L408)、[MainActivity.java:439](../build/android/app/src/main/java/com/wails/app/MainActivity.java#L439)。
- 当前问题：无法通过字节识别的 `/api/webpage/resource` 内容统一退回 `text/html`，丢失后端资源类型。
- 操作：加载包含外部 CSS、普通脚本、模块脚本、字体和图片的受控原文页面；检查 WebView 网络响应、控制台和页面效果。
- 通过标准：响应具有与实际资源相符的 MIME 类型；CSS 和模块脚本无类型拒绝错误；图片、字体等资源能按原类型使用。
- 当前证据：S；未做 WebView 视觉或网络面板验证。

### M09 — Android 规则导入可选择文件

- 定位：[RulesTab.vue:327](../frontend/src/components/modals/settings/rules/RulesTab.vue#L327)、[MainActivity.java](../build/android/app/src/main/java/com/wails/app/MainActivity.java)。
- 当前问题：规则导入使用 HTML 文件输入，WebView 没有对应的 `onShowFileChooser` 实现。
- 操作：点击规则导入，选择合法 JSON 并确认；再测试取消选择、非法 JSON 和超出既有限制的文件。
- 通过标准：能打开系统选择器并读到所选文件；合法规则按确认结果导入；取消不修改数据；非法输入显示现有错误提示，不覆盖原规则。
- 当前证据：S；系统文件选择器与导入整合验收未执行。

### M10 — 后台服务超时可正常退出

- 定位：[WailsForegroundService.java:61](../build/android/app/src/main/java/com/wails/app/WailsForegroundService.java#L61)、[main-android.go:199](../main-android.go#L199)。
- 当前问题：`dataSync` 前台服务持续保活，没有处理 Android 15 的服务运行时限。
- 操作：API 35 环境开启自动刷新后进入后台，达到平台超时；可在隔离模拟器按官方测试方法缩短时限，并记录配置与恢复情况。另测试回前台、关闭刷新、拒绝通知权限。
- 通过标准：超时后按平台要求停止服务，不出现未停止服务导致的进程异常；重新打开应用后数据正常；其他结束路径不遗留服务。
- 当前证据：S；未运行六小时场景或缩短时限的模拟器测试。

## 5. 响应式布局验收用例

### R01 — 工具栏溢出菜单可滚动

- 定位：[ArticleToolbar.vue:136](../frontend/src/components/article/ArticleToolbar.vue#L136)、[ArticleToolbar.vue:148](../frontend/src/components/article/ArticleToolbar.vue#L148)。
- 当前问题：全局捕获的滚动事件无差别关闭菜单，菜单自身滚动也触发关闭。
- 前置条件：手机宽度、较低可用高度，启用足够多的工具栏动作，使菜单出现滚动条。
- 操作：打开文章工具栏溢出菜单，触摸滚动到最后一项并执行；再滚动文章主体、旋转屏幕或调整窗口尺寸。
- 通过标准：菜单内部滚动不关闭菜单，最后一项可见且可操作；外部滚动或尺寸变化后菜单位置与关闭行为正确；菜单不被底栏或系统安全区遮挡。
- 当前证据：S、J；实际工具栏逻辑在菜单自身收到非冒泡 `scroll` 后关闭。CSS 和触摸行为需浏览器复验。

### R02 — 手机底部导航返回目标列表

- 定位：[ActivityBar.vue:110](../frontend/src/components/sidebar/ActivityBar.vue#L110)、[ArticleDetail.vue:152](../frontend/src/components/article/ArticleDetail.vue#L152)。
- 当前问题：导航只切换筛选、不清除当前文章；该文章仍在结果中时，详情层会继续覆盖手机列表。
- 操作：在手机列表布局阅读一篇收藏文章，点击底栏“全部”；确保该文章仍在目标列表第一页。另覆盖收藏、稍后阅读及断点切换。
- 通过标准：点击手机底栏后显示目标列表及正确选中项，旧详情不再覆盖列表；桌面分栏阅读行为保持原有约定。
- 当前证据：S；未执行真实浏览器导航回归。

## 6. Folo 插件验收用例

### F01 — 旧文章的远端状态能够回传

- 定位：[sync.go:235](../internal/folo/sync.go#L235)、[sync.go:463](../internal/folo/sync.go#L463)。
- 当前问题：以最新文章发布时间作为下一次拉取下限，无法追踪旧文章之后发生的状态变化。
- 前置条件：两个不同发布时间、已匹配并完成同步的条目；旧条目初始未读且已收藏，本地无待上传修改。
- 操作：在远端将旧条目标记已读并取消收藏，再同步；反向修改已读状态后再同步；最后增加本地与远端同时修改的冲突用例。
- 通过标准：旧文章的远端已读/未读和收藏/取消收藏变化均能回传；存在本地待上传修改时仍遵守“本地修改优先”的既有约定。
- 当前证据：S、G；本地 HTTP mock 已复现旧文章远端已读变化不回传。取消收藏有调用链证据，仍需单独回归；未访问真实 Folo 账号。

### F02 — 取消登录后旧回调不能恢复凭据

- 定位：[login.go:338](../internal/folo/login.go#L338)、[login.go:346](../internal/folo/login.go#L346)、[folo_handlers.go:303](../internal/handlers/folo/folo_handlers.go#L303)。
- 当前问题：旧回调的交换上下文独立于登录取消，完成后仍直接保存令牌。
- 操作：用可控延迟阻塞登录回调的令牌交换；调用退出登录并等其完成，再释放旧回调。另测试旧登录被新登录替换、过期以及旧响应晚于新会话保存的顺序。
- 通过标准：退出后持久化令牌保持清空；旧尝试不能覆盖新会话；账户 API、登录状态和前端显示与最终有效会话一致。
- 当前证据：S、G；临时 Go overlay 已复现取消后状态为 `idle`/`failed`，旧回调仍保存会话。未验证真实 Folo 回调。
- 说明：关闭设置面板不等同于后端已经取消登录，不能用关面板代替本用例中的取消动作。

### F03 — 超过 500 条映射时旧修改可推进

- 定位：[folo_sync_db.go:77](../internal/database/folo_sync_db.go#L77)、[sync.go:116](../internal/folo/sync.go#L116)。
- 当前问题：固定取最近更新的 500 条映射，无差异筛选或分页；本地修改不更新映射时间。
- 前置条件：501 条映射，最新 500 条均无变更，最旧一条有本地已读或收藏修改。
- 操作：按实现声明的批次预算连续同步，记录待处理集或游标推进；覆盖推送失败后重试。
- 通过标准：旧修改不会被无变更记录永久挤出处理范围，能够在有界批次内最终送达；失败的修改继续保持待同步。无需单次处理全部映射。
- 当前证据：S、G；501 条映射的 mock 用例连续两次推送均未处理旧修改。未验证真实 Folo 服务。

### F04 — 同发布时间分页不漏条目

- 定位：[sync.go:237](../internal/folo/sync.go#L237)；同时覆盖第 267、269 行的回填分页。
- 当前问题：下一页时间上限减一毫秒，跳过边界时间相同的剩余条目。
- 前置条件：101 条不同 ID、相同发布时间的远端条目，本地均有可匹配文章，分页上限为当前 100 条。
- 操作：完成一次同步，再次同步并核对全部 ID；另测试跨页同时间条目、亚毫秒边界和回填分页。
- 通过标准：所有可枚举条目均被匹配，无静默遗漏或无限重复第一页。若上游能力确实不能枚举完整，须保留未完成状态并明确记录限制，不能把漏条目的结果验收为完整成功。
- 当前证据：S、G；101 条同时间条目的 mock 用例仅处理 100 条，后续仍遗漏一条。
- 说明：把时间偏移改成正一毫秒不是验收结论，仍需验证超过一页同时间条目的完整性。

### F05 — 迟到的账户查询不覆盖新状态

- 定位：[FoloIntegrationSettings.vue:65](../frontend/src/components/modals/settings/plugins/FoloIntegrationSettings.vue#L65)。
- 当前问题：账户查询响应没有请求版本或会话校验，直接写入当前账户状态。
- 操作：延迟旧账户查询；完成手动令牌登录后释放旧 `401`；再测试退出登录后释放旧成功响应，以及账户 A 切换 B 后释放 A 的响应。
- 通过标准：界面始终保持最新会话操作的结果；旧响应不能清空新账户或恢复旧账户；导入、同步按钮状态正确。
- 当前证据：S、J；实际组件脚本配合延迟 fetch，已复现令牌登录成功后被旧 `401` 清空账户。未执行完整浏览器 E2E。

## 7. 已有检查与待补充验证

本节是审查会话的历史执行记录，不是本文生成时重新运行的结果，也不是修复版本的通过记录。

| 检查 | 已有结果 | 解释 |
| --- | --- | --- |
| 第一轮移动交互定向 Vitest | 9 个文件、47 项通过 | 不覆盖已发现的跨组件和原生整合缺陷 |
| 第一轮补充详情页等定向 Vitest | 5 个文件、15 项通过 | 与前一组有重叠，不相加作为总覆盖数 |
| 第二轮 Folo/响应式定向 Vitest | 8 个文件、24 项通过 | 不覆盖同步分页、账户迟到响应和菜单自身滚动反例 |
| `go test api_proxy.go api_proxy_test.go` | 通过 | 验证已有 APIProxy 用例，未证明取消链路有效 |
| `go test -timeout=5m ./internal/folo ./internal/handlers/folo` | 通过 | 现有用例未覆盖本次发现的边界和竞态 |
| 临时反例探针 | 观察到上述各项注明的失败行为 | 探针仅保留会话结论，修复时需要补入正式测试 |
| Android 真机/模拟器、多尺寸浏览器视觉回归 | 未执行 | 不能据此声明移动端整体验收通过 |
| 修复版本构建和复验 | 未执行 | 尚无修复版本 |

第二轮已执行的前端命令（工作目录 `frontend/`）：

```sh
npm run test:unit -- src/components/modals/settings/plugins/FoloIntegrationSettings.test.ts src/composables/ui/usePhoneLayout.test.ts src/composables/ui/useCoarsePointer.test.ts src/composables/ui/useResizablePanels.test.ts src/composables/ui/useArticleTableSplit.test.ts src/components/article/ArticleToolbar.test.ts src/components/common/BaseSelect.test.ts src/components/common/ContextMenu.test.ts
```

修复后应先运行对应回归用例及上述相关测试，再按改动范围执行前端构建、当前应用构建入口和适用的 server 构建。构建要求以 [AGENTS.md](../AGENTS.md)、[服务器说明](SERVER_MODE/README.md) 和当前 Taskfile 为准。

多尺寸视觉回归至少检查：列表与详情切换、侧栏与底栏、设置导航与表单、长文案、菜单最后一项、横竖屏安全区；保存尺寸和截图，不用 jsdom 的结果替代浏览器布局证据。

## 8. 逐项执行记录与完成条件

每个编号复制一条执行记录：

```text
验收编号：
修复提交 / 构建版本：
执行日期与执行人：
系统 / API / 浏览器 / WebView 版本：
视口尺寸 / 布局 / 语言 / 主题：
测试数据与前置状态：
实际步骤与结果：
回归测试名称及命令：
日志 / 截图 / 请求记录：
结论：通过 / 失败 / 未执行
未满足条件及影响：
```

本范围完成需要各项通过标准均有适用证据；正常路径与故障路径均复验，未引入已存数据或桌面/server 模式回归。条件不足的项目保留“未执行”或“失败”，明确未满足部分，不用现有测试通过替代最终验收。

## 9. 平台依据

- [Android WebView 原生桥接安全](https://developer.android.com/privacy-and-security/risks/insecure-webview-native-bridges)：M01。
- [ConnectivityManager API](https://developer.android.com/reference/android/net/ConnectivityManager)：M02。
- [Android Uri API](https://developer.android.com/reference/android/net/Uri)：M07。
- [MIME 类型说明](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/MIME_types)：M08。
- [WebChromeClient 文件选择回调](https://developer.android.com/reference/android/webkit/WebChromeClient)：M09。
- [Android 前台服务超时](https://developer.android.com/develop/background-work/services/fgs/timeout)：M10。

## 10. 修复执行记录（本轮）

本节按第 8 节模板记录本轮修复，未执行的验证项不计为通过。

### M02 — 声明支持的旧版 Android 可启动

```text
验收编号：M02
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：构建校验 compileSdk 35、minSdk 21；设备验证未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：registerDefaultNetworkCallback 为 API 24，minSdk 21；外层 catch (Exception)
     接不住它抛出的 NoSuchMethodError（Error 而非 Exception），旧机启动即崩。
  2) 完整 lint（`gradlew :app:lintDebug`）显示同类缺陷不止文档所点的一处，而是 8 处：
     - API 24：registerDefaultNetworkCallback（MainActivity，原 702 行）
     - API 23：checkSelfPermission/requestPermissions（相机，MainActivity）
     - API 23：getActiveNetwork（emitSystemSnapshot，界面加载路径）
     - API 23：setTorchMode（WailsBridge 手电筒）
     - API 23：checkSelfPermission ×2 / requestPermissions（WailsBridge 定位）
  3) 修复：NetworkCallback 按 Build.VERSION 分支（<24 用 registerNetworkCallback + NetworkRequest）；
     权限检查改用 ContextCompat.checkSelfPermission；requestPermissions 加 >=M 守卫；
     getActiveNetwork 加 >=M 分支，并为 API 21-22 新增 emitLegacyNetwork（读 NetworkInfo，
     保持与 emitNetwork 相同的事件结构，避免把在线误报为离线）；setTorchMode 加 <M 早退。
回归测试名称及命令：
  `gradlew.bat :app:compileDebugJavaWithJavac` → BUILD SUCCESSFUL
  `gradlew.bat :app:lintDebug` → NewApi 计数 0（修复前为 7，另加已修的 API 24 一处）
日志 / 截图 / 请求记录：lint 文本报告 lint-results-debug.txt（生成时间 12:11:46，NewApi 0 条）
结论：部分通过（代码与静态检查通过；设备冷启动未执行）
未满足条件及影响：本机仅安装 android-37 系统镜像，缺 API 21/23 镜像，未执行声明范围内的
  旧版系统冷启动与返回前台验证。该项在补测前不得记为通过。
```

### M07 — 资源请求查询串保真

```text
验收编号：M07
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：构建校验 compileSdk 35、minSdk 21；设备验证未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：Uri.getQuery() 返回解码后的查询串，重新拼成 path?query 再交给 Go 解析，
     会使内层 URL 中的 %26 变成外层参数分隔符、%23 截断整个查询、%2B/+ 语义互换。
  2) 修复：/wails/ 运行时请求与 /api/ 子资源两处均改用 Uri.getEncodedQuery()，保留原始编码；
     Go 侧按一次解码处理，语义与桌面端一致。
  3) 复核：MainActivity 内已无 .getQuery() 残留（计数 0）。
回归测试名称及命令：`gradlew.bat :app:compileDebugJavaWithJavac` → BUILD SUCCESSFUL
日志 / 截图 / 请求记录：无（未新增自动化用例）
结论：部分通过（代码修复与编译通过；目标 URL 保真的端到端验证未执行）
未满足条件及影响：尚未用 mock 记录经 WebView 实际到达 Go 的目标 URL，也未覆盖空格、中文、
  百分号编码与嵌套 iframe 路径。该项在补测前不得记为通过。
```

### M01 — 外部原文与原生桥接隔离

```text
验收编号：M01
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：构建校验 compileSdk 35；设备验证未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：原文视图在 ArticleDetail.vue 与 ArticleDetailModal.vue 中各有一个 iframe，
     src 为同源的 /api/webpage/proxy，且属性为
     sandbox="allow-scripts allow-same-origin allow-popups"。allow-scripts 与
     allow-same-origin 同时存在会取消沙箱：被代理的外部页面保持应用源，可访问
     window.parent、应用 storage，并经同源 fetch（已被 apiBridge 垫片接管）触达原生桥。
  2) 复核第二层：/api/ 子资源响应统一带 Access-Control-Allow-Origin: *，即使帧被隔离为
     不透明源，仍可跨源读取 /api/ 数据（Origin: null 与通配符匹配）。
  3) 修复：
     - 两处 iframe 改为 sandbox="allow-scripts allow-popups allow-forms" 并加
       referrerpolicy="no-referrer"：保留脚本渲染与外链，但降为不透明源，父文档、
       存储、原生桥均不可达；沙箱标记随同帧导航保留，覆盖「页面导航路径」。
     - /wails/ 运行时分支去掉通配 CORS（仅同源主界面消费）。
     - /api/ 子资源仅对 /api/webpage/ 保留 CORS，其余路由不再可跨源读取。
       保留代理路由的 CORS 是必要的：不透明源下字体与模块脚本需要 CORS，否则
       M08 的「字体、模块脚本按原类型可用」会同时失效。
  4) 同类旁证：XPathPicker.vue 的预览帧已是 sandbox="allow-scripts" 配合严格 CSP
     （default-src 'none'; connect-src 'none'），即项目既有正确形态，无需改动。
回归测试名称及命令：
  `npx vitest run src/components/article/ArticleDetail.test.ts` → 3 passed
  新增两条：详情页按渲染断言 sandbox 不含 allow-same-origin；
  卡片弹窗按源码守卫（?raw 导入）断言同一约束。
  反向对照：`git stash` 撤掉两处 .vue 修复后重跑，该用例失败；恢复后通过。
日志 / 截图 / 请求记录：无
结论：部分通过（代码、回归测试与反向对照通过；设备侧端到端未执行）
未满足条件及影响：尚未在真实 WebView 中加载受控原文页面，实测其无法访问
  window.parent、window.wails 与 /api/ 数据（含直接 iframe、嵌套 iframe、导航后三种路径），
  也未验证主界面合法通信（含代理页字体/模块脚本）在这些改动后仍正常。补测前不得记为通过。
```

### F02 — 取消登录后旧回调不能恢复凭据

```text
验收编号：F02
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Go 本地测试；真实 Folo 服务未访问
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：httptest 假 Folo 服务，交换响应受测试闸门控制
实际步骤与结果：
  1) 复核定位点：handleCallback 用 context.Background() 派生交换上下文（原注释明确写
     「detached」，故取消播不进去），交换成功后无条件调用 p.persist(ctx, token)。
     Cancel/CancelActiveLogin 只把状态记为 failed 并关闭监听，无法阻止在飞的交换。
     HandleLogout 的顺序是 CancelActiveLogin() 之后再清空令牌，因此存在两个竞态：
     (a) 退出后，旧回调把令牌写回；(b) 旧尝试晚于新登录提交，覆盖新会话。
  2) 修复（归属点 PendingLogin 的提交时机，而非调用方补丁）：
     - 新增 p.ctx（StartLogin 内由 serverCtx 赋值），交换改用其派生的带超时上下文：
       放弃尝试即取消请求，不再白跑网络。
     - 新增 persistIfCurrent：在 loginMu 内同时校验「仍是 activeLogin」与「未被放弃」
       后才写入。loginMu 也是 CancelActiveLogin 持有的锁，因此退出与提交互斥：
       要么令牌先写入、随后被退出清空，要么被拒绝，不存在中间态。
     - 回调据此返回「本次登录已取消」页面而不是成功页。
  3) 复核 persist 实现为 h.DB.SetEncryptedSetting，不回调登录状态机，持锁写入无死锁风险。
回归测试名称及命令：
  `go test -timeout=5m -run "TestCancelledLoginDoesNotPersistInFlightToken|TestPersistIfCurrentRefusesAbandonedOrReplacedAttempt" -v ./internal/folo/` → 2 passed
  TestCancelledLoginDoesNotPersistInFlightToken：交换被闸门挡住时取消，释放后断言未写入令牌、
  状态不为 connected。
  TestPersistIfCurrentRefusesAbandonedOrReplacedAttempt：已取消与被替换的尝试均被拒绝，
  当前尝试仍可写入，且最终只写入当前令牌。
  反向对照：临时把交换改回 detached 并短路提交闸门后重跑，两例均失败，报错为
  「a cancelled sign-in stored "SESSION-LATE"」，即原缺陷症状；恢复后文件字节一致。
  `go test -timeout=5m ./internal/folo/ ./internal/handlers/folo/` → ok / ok；`go vet` 通过。
日志 / 截图 / 请求记录：无
结论：部分通过（代码、两条回归与反向对照通过；真实回调与端到端未执行）
未满足条件及影响：未使用真实 Folo 账号验证回调时序，也未覆盖「账户 API、登录状态与前端显示
  与最终有效会话一致」这一条（其前端部分与 F05 的迟到响应问题重叠）。补测前不得记为通过。
```

### F05 — 迟到的账户查询不覆盖新状态

```text
验收编号：F05
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest + jsdom；浏览器端到端未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：fetch 桩；首个 /api/folo/account 响应被延迟到手动令牌登录完成之后
实际步骤与结果：
  1) 复核定位点：fetchAccount 在写 account / lastImport / lastSync / mappedCount 之前没有任何
     请求版本或会话校验，本地登录成功、登出、会话过期也都不会作废在飞的读取，因此一个在登录前
     发出的 401 晚到就会把刚连上的账户清空（文档所述反例）。
  2) 修复：引入 accountGeneration 单调代际号，每次读取在发请求时取号，写入前校验仍是最新一次；
     本地会话变更（手动登录成功、登出、expireSession）调用 invalidateAccountReads 使在飞读取作废。
     仅作废「读取」，不影响登录/登出自身的写入。
  3) 未改动 401/502 的既有语义：401 仍表示已登出，502 仍保留上次已知状态。
回归测试名称及命令：
  `npx vitest run src/components/modals/settings/plugins/FoloIntegrationSettings.test.ts` → 10 passed
  新增：`ignores an account read that was issued before a successful sign-in`
  反向对照：临时删除两处代际校验后重跑，该用例失败（同文件另有一条时序相关用例同时失败），
  恢复后文件字节一致，`npx vitest run .../plugins/` → 11 passed。
日志 / 截图 / 请求记录：无
结论：部分通过（代码与回归通过；完整浏览器 E2E 未执行）
未满足条件及影响：自动化只覆盖「登录成功后旧 401 迟到」这一条；「退出登录后旧成功响应迟到」与
  「账户 A 切到 B 后 A 的响应迟到」两个路径由同一代际守卫结构性覆盖，但没有各自独立的用例，
  也未在真实浏览器中验证导入/同步按钮状态。补测前不得记为通过。
```

### M06 — Android API 请求取消有效

```text
验收编号：M06
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Go 本地测试 + Vitest/jsdom；真实 JNI 未观测
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：httptest.ServeMux 中会阻塞到 ctx 结束的处理器；前端用替身 Call.ByName
实际步骤与结果：
  1) 复核定位点：apiBridge 的 fetch 垫片完全没有读取 init.signal——既不预检已中止，也不与
     AbortSignal 竞争，更不向 Go 传达取消。Go 侧 Do 用 httptest.NewRequest（隐式
     context.Background()）同步调用 mux，处理器无法感知取消。
     后果：切换文章后失效的翻译请求继续跑完，并继续占用翻译队列 6 个并发位之一。
  2) 修复：
     - 前端：每次调用生成 request_id；派发前检查 signal.aborted；随后把原生调用与 abort 竞速，
       中止时调用 main.APIProxy.Cancel(request_id) 并立即以 DOMException('AbortError') 拒绝，
       与真实 fetch 的取消形状一致，调用方的既有处理无需改动。
     - Go：APIProxyRequest 增加 request_id；Do 按 id 创建可取消 context 并挂到请求上；
       新增 Cancel(request_id)。Cancel 先到会记下意图（上限 256，超出即清空），使随后到达的
       同 id 请求直接以 errAPIProxyCancelled 返回、根本不派发。
     - 未改变空 request_id 的既有行为，非取消调用方不受影响。
  3) 队列侧复核：translationRequest 已具备 job.controller.signal、取消时移出队列、finally
     释放并发位；既有用例 `aborts in-flight requests after leaving the article` 覆盖该逻辑。
     之前缺的是底层真的会中止，现在成立。
回归测试名称及命令：
  `go test -v -run TestAPIProxy api_proxy.go api_proxy_test.go` → 4 passed
  新增：TestAPIProxyCancelStopsAnInFlightRequest（处理器观测到 ctx 结束且 Do 返回）、
  TestAPIProxyCancelBeforeRequestSkipsHandler（先取消则不派发）。
  `npx vitest run src/utils/apiBridge.test.ts` → 3 passed
  新增：已中止不发原生调用、处理中中止会以同 id 调用 Cancel 并以 AbortError 拒绝、request_id 透传。
  反向对照：`git stash` 撤掉 apiBridge.ts 后重跑，3 例全部失败（其中「处理中中止」用例 5s 超时，
  说明旧实现确实不会返回）；恢复后通过。
日志 / 截图 / 请求记录：无
结论：部分通过（前后端代码与 7 条回归通过；设备侧未执行）
未满足条件及影响：未在真实 WebView/JNI 中观测取消是否到达 Go（本机无 API 21/23 镜像与外层桥探针），
  也未实测「阻塞三个旧文章翻译后切换文章、新文章可继续翻译」这一端到端场景。补测前不得记为通过。
```

### M08 — 原文资源 MIME 类型正确

```text
验收编号：M08
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：compileSdk 35 / minSdk 21；WebView 视觉验证未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：sniffMimeType 只做字节嗅探，失败后对 /api/webpage/ 一律回退 text/html。
     CSS、module script、字体都没有魔术字节，因此被当成 HTML 返回，WebView 会拒绝应用
     （样式不生效、模块脚本报类型错误、字体不加载）。
  2) 传输层限制确认：serveAsset 只回传字节，不带上游 Content-Type，桥接无法透传真实类型，
     因此只能在 Java 侧推断；推断源取同一事实源——url_b64 中的原始 URL 扩展名，而不是瞎猜。
  3) 修复：嗅探仍优先（PNG/JPEG/GIF/PDF 等二进制更可信）；嗅探失败时解析 url_b64（还原
     '+' 被解码成空格的情况）并按扩展名映射 css/mjs/js/json/woff2/woff/ttf/otf/svg/webp/
     avif/wasm/xml；两者都没有才回退原有的 text/html（代理文档）与 application/json。
     charset 规则不变：text/* 与 json/javascript/xml 才声明 UTF-8。
回归测试名称及命令：
  `gradlew.bat :app:testDebugUnitTest` → ResourceMimeTest tests=4 failures=0
  为了能写下这条回归，本轮为 Android 模块补了最小 JVM 测试位：app/build.gradle 加
  testImplementation junit:junit:4.13.2，并把扩展名映射抽到无 Android 依赖的
  com.wails.app.ResourceMime（MainActivity.mimeFromFileName 改为委托它），使纯逻辑可在
  普通 JVM 上验证；未引入 Robolectric。
  覆盖：css/mjs/js/json/woff2/woff/ttf/otf/svg/webp/avif/wasm/xml 的映射；查询串与片段
  不干扰扩展名；大小写不敏感；无法识别时返回 null（由调用方保留兜底）。
  反向对照：把 css 映射临时改成返回 null 后重跑，ResourceMimeTest 两条用例失败；恢复后
  文件字节一致。
  `gradlew.bat :app:assembleDebug` → BUILD SUCCESSFUL
  完整 lint → NewApi 0 条（原有 7 条延续 M02 修复），总 Error 9→2（余下两条为既有的
  LocationManager MissingPermission 误报，不属于本清单）。
日志 / 截图 / 请求记录：无
结论：部分通过（代码、静态检查与 4 条 JVM 回归通过；WebView 验证未执行）
未满足条件及影响：未在真实 WebView 中加载含 CSS、模块脚本、字体与图片的受控原文页面，
  确认没有被拒绝的类型错误（即映射之外还有 WebView 实际行为这一层）。补测前不得记为通过。
```

### M09 — Android 规则导入可选择文件

```text
验收编号：M09
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：compileSdk 35 / minSdk 21；设备验证未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：RulesTab 用 <input type="file" accept=".json"> 加脚本点击，而 MainActivity
     从未调用 setWebChromeClient，WebView 没有 onShowFileChooser，点击后没有任何反应。
     注意这与既有的 FILE_PICKER_REQUEST（Go 侧主动拉起的原生选择器）不是同一条路径。
  2) 修复：安装 WebChromeClient，在 onShowFileChooser 中用 params.createIntent() 拉起系统
     选择器并补 CATEGORY_OPENABLE；以 WEB_FILE_CHOOSER_REQUEST(7011) 独立结果码区分于既有的
     文件选择与拍照路径；onActivityResult 中用 FileChooserParams.parseResult 回填，
     取消时回传 null 以免页面的 input 永久挂起；重复打开前先释放上一个回调。
回归测试名称及命令：`gradlew.bat :app:compileDebugJavaWithJavac` → BUILD SUCCESSFUL；lint NewApi 0
日志 / 截图 / 请求记录：无
结论：部分通过（代码与编译通过；选择器与导入整合未验证）
未满足条件及影响：未在设备上确认系统选择器能弹出并读回所选 JSON，也未覆盖取消选择、非法 JSON、
  超限文件三条路径；Android 模块无 JVM 测试设施，本轮无自动化回归。补测前不得记为通过。
```

### M10 — 后台服务超时可正常退出

```text
验收编号：M10
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：compileSdk 35 / minSdk 21；API 35 超时场景未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核定位点：WailsForegroundService 只实现了 onStartCommand（START_STICKY）与 onBind，
     没有 onTimeout。Android 15 对 dataSync 型前台服务有运行时限，到时未停止会被系统按
     「未按时停止前台服务」终止进程。
  2) 修复：覆写 onTimeout(int startId, int fgsType) 并立即 stopSelf(startId)。后台刷新本身
     是无状态设计（切后台启动、回前台停止），因此停止后不需要额外状态回滚；下一次进入后台
     会重新拉起新的服务。
  3) 未改动 START_STICKY：主动 stopSelf 不触发系统重启，只有被系统杀死且仍被需要时才会重启。
回归测试名称及命令：`gradlew.bat :app:compileDebugJavaWithJavac` → BUILD SUCCESSFUL；lint NewApi 0
日志 / 截图 / 请求记录：无
结论：部分通过（代码与编译通过；超时行为未在设备验证）
未满足条件及影响：未按官方方式缩短时限（device_config）实测超时后服务停止且无进程异常，也未复验
  「回前台停止、关闭刷新停止、拒绝通知权限不启动」三条既有路径在这些改动后仍成立。
  补测前不得记为通过。
```

### M03 — 文章返回保持原列表范围

```text
验收编号：M03
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest/jsdom；系统返回的设备整合未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核根因：useAndroidBack 的优先级梯子里 articleDetail=0、view=5，而两个处理器在
     文章打开时同时注册。view 处理器只要 scope 非 all 就返回 true 并执行 setFilter('all')，
     因此按返回时它先于文章处理器命中——正是「先从订阅/分类/未读/收藏打开文章，按返回却跳到
     All」的现象。ArticleDetail 的处理器本来就在无文章时返回 false，问题只在梯子把文章放在了
     列表范围之下（注释还写着「文章是最底层」）。
  2) 修复：articleDetail 0→8，即位于 view(5) 之上、drawer(10) 之下——文章覆盖它被打开的那个
     列表，必须先消费返回；抽屉与各 sheet 仍在文章之上。同步修正 useAndroidBack 与
     ArticleDetail 里「文章是最底层」的过时注释，避免把错误语义再传给下一个人。
  3) 未改 view 处理器本身：无文章时文章处理器返回 false，重置仍照旧生效。
回归测试名称及命令：
  `npx vitest run src/composables/ui/useAndroidBack.test.ts` → 7 passed
  新增两条：`closes an open article before resetting the list scope`（区分性证据）、
  `still resets the list scope when no article consumed the press`（兜底回归保护）。
  反向对照：把 articleDetail 改回 0 后，第一条用例失败（期望 close-article，实际 reset-scope）。
  说明：第二条在同一批跑中也会失败，但那次失败是第一条断言中断导致组件未 unmount、注册泄漏
  污染了后续用例；单独运行该用例在改回 0 后仍然通过（1 passed）。因此第二个用例不作为修复
  的区分性证据，只作为「提高优先级没有破坏普通列表返回」的保护。
  `npx vitest run` → 69 files / 328 tests passed。
日志 / 截图 / 请求记录：无
结论：部分通过（优先级与回归通过；设备系统返回未执行）
未满足条件及影响：文档要求的验收是设备上按 Android 系统返回键，分别从订阅、分类、未读、收藏
  四个入口验证「第一次关闭文章并保留原列表范围」，再在列表页验证后续返回。本轮只验证了调度
  层级，未做设备整合；图片查看器与查找面板（650/700）本就高于文章，未改动也未单独复验。
  补测前不得记为通过。
```

### M04 — 卡片阅读弹窗响应系统返回

```text
验收编号：M04
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest/jsdom；系统返回的设备整合未执行
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：无
实际步骤与结果：
  1) 复核根因：ArticleDetailModal 只在 window 上监听 keydown 处理 Escape，从未调用
     useAndroidBack。因此卡片布局下按系统返回时，弹窗不在调度栈里：要么落到 M03 的列表范围
     重置，要么一路穿透并退出应用，文章弹窗仍然开着——正是文档描述的两种失败。
  2) 修复：把 Escape 的逐层关闭抽成 dismissTopLayer（查找面板 → 图片查看器 → 关闭弹窗），
     供键盘与系统返回共用，避免两条路径的关闭顺序日后走偏；以 BACK_PRIORITY.dialog(500)
     注册：高于列表范围(5)与文章(8)，低于查找面板(650)与查看器(700)，即「逐层关闭最上方界面」。
  3) 未改动 ArticleList 的 @close 连线与卡片弹窗开关逻辑。
回归测试名称及命令：
  `npx vitest run src/components/article/ArticleDetailModal.test.ts` → 2 passed
  新增：`closes the dialog when the system back press arrives`（真实挂载，断言调度返回 true
  且 emit('close') 一次）、`does not answer once unmounted`（卸载后不得再吞按键）。
  反向对照：临时删掉注册语句后重跑，第一条失败（expected false to be true），第二条不受影响；
  恢复后文件字节一致。
  `npx vitest run` → 70 files / 330 tests passed；`npm run build` → 成功（✓ built in 3.68s）。
日志 / 截图 / 请求记录：无
结论：部分通过（注册、逐层关闭与回归通过；设备系统返回未执行）
未满足条件及影响：文档还要求覆盖「弹窗内查找、图片查看器、图库媒体弹窗」三层在设备上逐层关闭；
  本轮只验证了调度优先级与关闭事件，图库媒体弹窗（同类路径）未复验，也未在真机上确认
  「打开弹窗时按返回不会直接退出应用或切换后台列表范围」。补测前不得记为通过。
```

### M05 — 抽屉内弹窗不被外部点击处理销毁

```text
验收编号：M05
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest/jsdom；完整界面 E2E 未执行
视口尺寸 / 布局 / 语言 / 主题：jsdom 默认视口，未做手机宽度视觉验证
测试数据与前置状态：无
实际步骤与结果：
  1) 复核根因：Sidebar 的 collapseFloatingFeedList 在 document 上以捕获阶段监听
     pointerdown/focusin/blur，判据只有「目标是否在 sidebarRoot 内」。而 FeedList 把保存/编辑
     筛选的 SavedFilterModal 用 Teleport 挂到 body，其 DOM 天然不在 sidebarRoot 内——于是第一次
     在弹窗里按下鼠标或聚焦输入框，抽屉就被折叠，FeedList 随之卸载，弹窗一起消失。
  2) 修复：抽屉自己 Teleport 出去的对话框带 data-drawer-popover 标记（保存与编辑两处都加），
     抽屉判断外部点击时把这类节点整体视为内部。标记落在 BaseModal 的根 backdrop 上
     （BaseModal 单一根节点且未关闭属性透传），因此点击弹窗内容与背板都不会折叠抽屉；
     背板点击仍由 BaseModal 自身的 closeOnClickOutside 关闭弹窗，符合「外部点击关闭对应层」。
  3) 未改 pinned 与 isOpen 的既有短路：桌面固定列不受影响。
回归测试名称及命令：
  `npx vitest run src/components/sidebar/Sidebar.test.ts` → 6 passed
  新增：`keeps the sheet for a dialog Teleported out of the drawer`（行为层：标记内 pointerdown
  不触发 toggle，真正外部点击仍触发一次）、`marks every drawer-owned dialog as belonging to the
  drawer`（标记层：FeedList 中至少两处标记，防止只改一处）。
  反向对照：分别删除 Sidebar 的标记判断、删除 FeedList 的两处标记后重跑，各自对应用例失败；
  两个文件恢复后字节一致。
  `npx vitest run` → 70 files / 332 tests passed；`npm run build` → 成功（✓ built in 2.71s）。
日志 / 截图 / 请求记录：无
结论：部分通过（事件判据与标记回归通过；完整界面 E2E 未执行）
未满足条件及影响：文档要求「保存能完成且重新打开后内容正确」，即筛选保存的持久化往返；本轮只
  验证了事件不会被误判，没有走保存接口与重新打开的完整流程，也没有手机宽度下的真实抽屉交互。
  补测前不得记为通过。
```

### R01 — 工具栏溢出菜单可滚动

```text
验收编号：R01
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest/jsdom；真实浏览器与触摸未执行
视口尺寸 / 布局 / 语言 / 主题：jsdom 视口；手机宽度视觉未验证
测试数据与前置状态：手机断点匹配已桩；底栏高度以 56px 注入
实际步骤与结果：
  1) 复核根因：scroll 监听注册在 window 上且 capture=true，因此菜单面板自身的滚动也会进入
     onViewportChange，而该函数无条件 closeMenu——菜单一滚就关，"滚到最后一项并执行"无法完成。
  2) 第二处（遮挡）：openMenu 的 maxHeight 只按 innerHeight - top - 8 计算，菜单可一直延伸到
     视口底并压住固定底栏（底栏 height 为 calc(56px + env(safe-area-inset-bottom))）。
  3) 修复：
     - onViewportChange 收到事件后，若 target 在菜单面板内则忽略；resize 与外部滚动仍关闭菜单。
     - openMenu 在手机断点下测量 .smart-activity-bar 的 offsetHeight 并从可用高度中扣除，
       不复制 56px/safe-area 这条数值；桌面布局不测量（固定列占满高度，不能扣）。
回归测试名称及命令：
  `npx vitest run src/components/article/ArticleToolbar.test.ts` → 5 passed
  新增：`keeps the overflow menu open while the menu itself scrolls`（面板 scroll 后 5 项仍在；
  页面滚动后菜单关闭）、`stops the overflow menu above the phone bottom bar`（maxHeight 必须
  扣除注入的 56px 底栏）。
  反向对照：删除面板内判据后第一条失败；把 reservedBottom 改回 0 后第二条失败；恢复后字节一致。
  `npx vitest run` → 71 files / 336 tests passed；`npm run build` → 成功（✓ built in 3.18s）。
日志 / 截图 / 请求记录：无
结论：部分通过（两处代码与回归通过；浏览器/触摸未执行）
未满足条件及影响：文档还要求「旋转屏幕或调整窗口尺寸后菜单位置与关闭行为正确」以及
  「不被底栏或系统安全区遮挡」。本轮只按视口尺寸计算与 jsdom 断言，未在真实设备上验证旋转、
  安全区（env 内边距实际取值）与触摸滚动；`Math.max(160, ...)` 的 160px 下限在极小可用高度下
  仍可能压住底栏，这一点未复验。补测前不得记为通过。
```

### R02 — 手机底部导航返回目标列表

```text
验收编号：R02
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Vitest/jsdom；真实浏览器导航未执行
视口尺寸 / 布局 / 语言 / 主题：jsdom 视口
测试数据与前置状态：store 替身中 currentArticleId 预置为 7
实际步骤与结果：
  1) 复核根因：ActivityBar 的 handleNavClick 只做 clearAllFilters + setFilter + 选中回调，
     从不清除 store.currentArticleId。手机布局下文章详情是覆盖列表的整层，因此点了底栏目标
     筛选后，旧详情仍盖在上面，用户看不到"正确选中项"。
  2) 修复：在手机断点（PHONE_QUERY，与 App/ArticleDetail/Sidebar 同一常量）下把
     currentArticleId 置空，详情层随之关闭；桌面分栏不进入该分支，保留"切筛选仍在读这篇"的
     既有约定。
  3) 未改动 setFilter 的加载时序与 clearAllFilters 语义。
回归测试名称及命令：
  `npx vitest run src/components/sidebar/ActivityBar.test.ts` → 2 passed
  新增：`leaves the open article when a filter is chosen on a phone`（手机：currentArticleId
  变为 null 且 setFilter 被调用）、`keeps the split-layout reading pane intact`（桌面：仍为 7）。
  反向对照：把置空语句改成 void 0 后，手机用例失败；恢复后字节一致。
  `npx vitest run` → 71 files / 336 tests passed；`npm run build` → 成功。
日志 / 截图 / 请求记录：无
结论：部分通过（代码与回归通过；真实浏览器导航未执行）
未满足条件及影响：未在真机上覆盖「收藏、稍后阅读及断点切换」三条路径，也未验证目标列表首屏
  仍包含该文章时点击后列表是否滚到该项（文档要求"显示目标列表及正确选中项"）。补测前不得记为通过。
```

### F04 — 同发布时间分页不漏条目

```text
验收编号：F04
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Go 本地测试；真实 Folo 服务未访问
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：101 条 ID 不同、PublishedAt 完全相同的远端条目；游标固定在 2026-10-01
实际步骤与结果：
  1) 复核根因：分页只有时间边界（EntryListParams.NewerThan/OlderThan → publishedBefore/
     publishedAfter），下游用 shiftTime(oldest, -1ms) 推进。当一个发布时间内的条目多于单页
     （100）时，第二页用比该时间早 1ms 的上界去取，取不到同一时间戳的剩余条目，返回空页；
     原逻辑据此判定 liveCaughtUp 并宣告拉取完成，第 101 条被静默遗漏且结果看起来是成功的。
  2) 关键限制确认：上游没有 id 游标或 offset，同一时间戳内部无法分页。因此按文档允许的第二种
     处理方式执行——保留未完成状态并明确记录限制，不把漏条目验收为完整成功。
  3) 修复：
     - 检测：整页结束且该页边界时间戳出现多于一次（countPublishedAt > 1）时，若下一页为空，
       判定分页切进了单一发布时间的条目带内，标记 truncated 与该时间戳；对「返回同一整页」
       的另一种上游行为也做了同样标记（added==0 分支）。回填分页同样处理。
     - 记录：SyncResult 增加 Truncated / TruncatedAt，并写入设置 folo_sync_truncated_at，
       后续每次同步都会继续报告该缺口（缺口不会因下一次同步而消失）。
     - 推进：游标仍推进到该边界时间戳，避免同一页在单次同步内被反复读取（原缺陷的另一半）。
回归测试名称及命令：
  `go test -timeout=5m -run TestSyncReportsTruncatedCoverageForOnePublishTimeOverflow -v ./internal/folo/` → PASS
  断言：单次同步只处理一页（Skipped=100）、结果 Truncated=true 且 TruncatedAt 等于该时间戳、
  时间线请求共 3 次（两页 + 收藏集合页，未循环）、游标推进到边界；第二次同步仍报告同一缺口。
  反向对照：把边界判定改成恒 false 后重跑，用例失败并停在
  「a publish time holding more entries than a page must be reported as truncated」；恢复后字节一致。
  `go test ./internal/folo/ ./internal/handlers/folo/` → ok / ok；`go vet` 通过；`go build ./...` 通过。
日志 / 截图 / 请求记录：无
结论：部分通过（按「记录限制」分支实现并有回归；「全部可枚举条目均被匹配」分支未达成）
未满足条件及影响：第 101 条在同一次同步中无法被枚举——上游没有游标，这是能力限制而非本次实现
  缺陷，因此该项不能记为完全通过。现实影响是：与该时间戳同批、落在第 101 位及之后的条目的已读/
  收藏状态不会被同步，直到上游提供可切分该带的游标。测试也未接真实 Folo 服务，未覆盖「跨页同
  时间条目」「亚毫秒边界」与「回填分页遇到同一时间带」三组场景（代码路径已按同一判据处理）。
```

### F03 — 超过 500 条映射时旧修改可推进

```text
验收编号：F03
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Go 本地测试；真实 Folo 服务未访问
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：501 条映射（同一 feed 下 501 篇文章），批次预算 500
实际步骤与结果：
  1) 复核根因（两层）：
     - ListFoloMappedArticles 用 ORDER BY updated_at DESC LIMIT 500，每次取「最近触碰」
       的 500 条；第 501 条永远在窗口外，它的本地修改无法被推送。
     - 即便改成升序，写入用的是 time.Now().Unix()（秒）。一次批次会为 500 行写同一个
       时间戳，升序在平局下仍可能反复选中同一批，轮转不成立。
  2) 修复：
     - 批次改为 ORDER BY m.updated_at ASC, m.article_id ASC：已处理的行由
       UpdateFoloSnapshot 打上新时间戳移到队尾，未处理的行自然前移，整个表在有界批次内轮转。
     - 时间戳改用 UnixMilli，消除同秒平局。旧数据是秒级，数值更小、排序更靠前，升级后第一轮
       就会把它们重写为毫秒，单位自然收敛（已在函数注释中记录这条迁移语义）。
  3) 未改动推送失败的处理：失败的行不会写新快照，因此保持原位置，下一轮仍会被取到。
回归测试名称及命令：
  `go test -timeout=5m -run TestListFoloMappedArticlesRotatesOlderMappingsIntoTheBatch -v ./internal/database/` → PASS
  断言：501 条映射下连续两轮 500 条上限的批次必须覆盖全部 501 条（第一轮打戳把 500 条推到
  队尾，被挤出的那条在第二轮进入窗口）。
  反向对照：把排序改回 updated_at DESC 后重跑，用例失败并报
  「two bounded passes covered 500 of 501 mappings」，即原缺陷症状；恢复后字节一致。
  `go test ./internal/folo/ ./internal/database/ ./internal/handlers/folo/` → 三个包均 ok；
  `go vet` 通过。
日志 / 截图 / 请求记录：无
结论：部分通过（批次轮转与回归通过；推送端到端未执行）
未满足条件及影响：用例停在数据库批次层，没有驱动 Syncer 走完「检测本地修改 → 推送失败 →
  下一轮重试」的完整链路，因此「失败的修改继续保持待同步」只由代码路径保证、未由用例固定；
  也未访问真实 Folo 服务验证 501 条规模下的实际行为。补测前不得记为通过。
```

### F01 — 旧文章的远端状态能够回传

```text
验收编号：F01
修复提交 / 构建版本：工作区未提交（feat/folo-integration，基于 9afd4efa）
执行日期与执行人：2026-10-24 / agent
系统 / API / 浏览器 / WebView 版本：Go 本地测试；真实 Folo 服务未访问
视口尺寸 / 布局 / 语言 / 主题：不适用
测试数据与前置状态：遵守时间边界的假时间线（timelineFeed）+ 一条按 URL 匹配的本地文章
实际步骤与结果：
  1) 复核根因：live 拉取只读「比游标更新」的条目（sync.go 的 NewerThan），旧文章在发布之后
     发生的已读/收藏变化不会再进入任何一次查询窗口。接口只有 publishedBefore/publishedAfter
     与 isCollection 三个维度，没有按 ID 取条目、也没有「最近变更」端点。
  2) 已落地（未验证）：
     - 映射表查询带上 published_at；新增 ListFoloMappingsForRecheck(afterID, limit)，按
       article_id 水位轮转，刻意不复用 updated_at，避免与 F03 的推送窗口抢位。
     - Syncer.recheckRemote：每轮对至多 8 条已映射条目，按各自发布时间开一个
       (ts-1ms, ts+1ms] 的窄窗复查，命中同 ID 条目后走既有 applyEntry，因此「本地修改优先」
       的约定不变，且一次复查同时带回 read 与 collections 两个状态。
     - shiftTime 改为多布局容错：published_at 的入库格式因路径而异，解析失败会让两个边界相等，
       把复查窗口退化成空窗口。
  3) 定位与修复（本轮）：探针确认 `articles.published_at` 实际存的是 Go `time.Time.String()`
     形式（`2026-10-20 09:00:00 +0000 UTC`），既不是 RFC3339 也不是 SQLite 常见形式，
     因此 shiftTime 解析失败、两个边界相等，复查窗口退化成空——机制形同虚设。shiftTime 增加
     该布局（含/不含小数秒）后复查才真正发出。
  4) 用例自身的缺陷也一并修正：只放一条旧条目时，游标恰好停在它自己的发布时间上，live 窗口
     仍会重读它，用例无法区分「复查生效」与「live 兜住」。改为再加一条更新的条目把游标推过去，
     之后只有复查窗口能到达旧条目。
回归测试名称及命令：
  `go test -timeout=5m -run TestSyncRechecksOldEntryReadState -v ./internal/folo/` → PASS
  断言：首轮同步采纳为已读且已收藏；远端把该旧条目改回未读并移出收藏后，第二轮同步本地必须
  同时变回未读且未收藏；并断言发出过 `(ts-1ms, ts+1ms]` 的复查窗口（limit=8）。
  反向对照（两个，分别针对一侧）：
  - 让复查跳过全部条目 → 报「an old entry changed remotely stayed stale locally」并附未发出
    复查窗口，证明已读/未读与关闭窗口的关联依赖复查；
  - 让夹具不再输出 collections → 报「after the first pull: read=true fav=false, want both
    true」，证明收藏状态只有在行结构与真实接口一致时才可观测，即该断言不是凭空为真。
  恢复后两处均字节一致。`go test ./internal/...` → 45 个包通过；`go vet ./internal/...` 通过。
日志 / 截图 / 请求记录：无
结论：通过（代码与回归；真实 Folo 服务未接入，见下）
未满足条件及影响：仍未访问真实 Folo 账号，因此「真实服务返回的 collections 结构与此处夹具
  一致」这一前提未经外部验证；复查每轮固定最多 8 条，映射超过该数量时旧条目要靠多轮才能轮完，
  这一时长特性未在真实账号上度量。上述不影响本次用例覆盖的行为，但属于未验证范围。
```

### 设备验证记录（第 15 轮）

```text
环境：Pixel_10 AVD / Android API 37 / app-debug.apk（41MB）/ 无窗口模式
安装与启动：`adb install -r` → Success；`am start -n com.mrrss.app/com.wails.app.MainActivity`
  注意 applicationId 为 com.mrrss.app（namespace 是 com.wails.app），用后者启动会得到 Error type 3。
WebView 调试：`adb forward tcp:9222 localabstract:webview_devtools_remote_<pid>` 后可列出目标页
  http://wails.localhost/。
证据文件：docs/evidence/android-launch.png、android-article-detail.png、android-after-back.png

已取得设备证据的观察：
  1) 应用在 API 37 上安装、启动、渲染正常：底部五个视图、未读角标 3、收藏星标、中文标题
     「所有文章」均正确显示（android-launch.png）。
  2) 手机布局下点开文章进入全屏详情（标题、摘要卡片、正文、底部「下一篇文章」），底栏仍浮在
     详情之上（android-article-detail.png）。
  3) 详情层按一次返回键回到列表，应用进程 pid 未变（5163）、Activity 仍在前台，未退出到桌面
     （android-after-back.png + pidof/dumpsys window 输出）。这条对应 M03/M04 的其中一层。

未取得设备证据（本轮仍未执行）：
  - 返回键的其余层级（抽屉、底部弹层、对话框、查找栏、图片查看器、媒体弹窗）未逐层验证。
  - M02：本机只有 API 37 镜像，API 21/23 的兼容分支无法在此验证，按要求不拿高版本结果冒充。
  - M01（沙箱 iframe 运行时行为）、M07（查询参数）、M08（MIME 嗅探）、M09（文件选择器）、
    M10（前台服务超时）均需要构造受控页面或特定系统事件，本轮未执行。
```

### 设备验证记录（第 16 轮，抽屉层）

```text
证据文件：docs/evidence/android-drawer.png、android-after-drawer-back.png
操作：在列表页点左上汉堡 → 抽屉展开 → 截屏 → 按一次返回 → 截屏
结果：
  1) 抽屉展开正常：标题「订阅源」、搜索框、排序与关闭按钮、未分类(3)、Fixture News(1)、
     Fixture Space(2)；底部为「+ 添加订阅」与「设置」。
     这同时验证了中途那次设计修正的落地：添加订阅与设置确实移入抽屉底部，底栏只剩五个视图。
  2) 抽屉层按返回只关闭抽屉，回到列表；进程 pid 仍为 5163，Activity 仍在前台，未退出。
     对应 M04 返回阶梯中的「抽屉」一层。
仍未执行：底部弹层、对话框、查找栏、图片查看器、媒体弹窗各层；M02 因缺 API 23 镜像；
  M01/M07/M08/M09/M10 需受控场景。
```

### 设备验证记录（第 17 轮，工具栏弹层与新发现）

```text
证据文件：docs/evidence/android-toolbar-menu.png、android-after-menu-back.png
操作：列表点开文章 → 点工具栏「⋯」→ 截屏 → 按一次返回 → 截屏
结果：
  1) 弹层在设备上打开正常：菜单锚定在触发按钮下方、完整落在视口内，五项为「从稍后阅读中移除 /
     在浏览器中打开 / 复制标题 / 复制链接 / 重新加载文章内容」。这是 R01 的设备前提。
  2) 新发现（未修复）：弹层没有拦截返回键。一次返回直接关闭了文章详情回到列表，而弹层此刻
     仍处于打开状态——它随详情一起被卸载，等于"返回穿透了弹层"。
     根因方向：ArticleToolbar 的溢出菜单只处理 Escape 与外部点击，没有注册 useAndroidBack，
     因此不会在 BACK_PRIORITY 阶梯上占位。
     该行为不在 M03/M04 明列的层级（查找栏、图片查看器、图库媒体弹窗）之内，但不符合
     "先关最上层"的通用约定。第 18 轮已修复，见下。
仍未执行：底部弹层、对话框、查找栏、图片查看器、媒体弹窗各层的返回验证。
```

### 修复记录（第 18 轮）：工具栏弹层拦截返回

```text
修复位置：frontend/src/components/article/ArticleToolbar.vue
改动：给溢出菜单注册 useAndroidBack(handler, BACK_PRIORITY.popover)。菜单打开时消费返回并
  closeMenu(true) 归还焦点；菜单关闭时返回 false 让位给下一层（文章详情），因此不改变原有行为。
回归测试：`npx vitest run src/components/article/ArticleToolbar.test.ts` → 6 passed
  新增 `closes the overflow menu on back before the article`：菜单打开时调用桥接入口
  window.__mrrssHandleBack()，断言返回 true 且菜单节点消失。
反向对照：把处理函数的「菜单未打开则让位」改为恒让位后重跑，用例失败于
  `expected false to be true`（即返回未被消费），恢复后字节一致。
全量：`npx vitest run` → 71 文件 / 337 用例通过；`npm run build` 成功；
  `:app:assembleDebug` 成功。
未完成：设备复验未做。本轮 `:app:assembleDebug` 仅耗时 2 秒，未确认重新嵌入了新的前端产物，
  直接重装可能仍是旧行为，因此不以重装结果作为证据；设备复验需要在执行完整的 Wails 前端
  构建后再进行。
```

### 17 项状态汇总（第 19 轮核对）

```text
代码侧基线：go test ./internal/... → 45 个包通过 / 0 失败；go vet ./... → 通过；
            npx vitest run → 71 文件 / 337 用例通过；npm run build → 成功；
            :app:assembleDebug → 成功；模拟器 Pixel_10 (API 37) 安装并启动成功。

编号  项                              代码  回归测试                 设备证据        结论
M01   沙箱 iframe / CORS 收窄          已改  有（渲染期属性断言）      未执行          部分通过
M02   API 21/23 兼容                  已改  无（框架分支）            未执行（缺镜像） 部分通过
M03   返回阶梯优先级                  已改  有                       详情层、抽屉层   部分通过
M04   弹窗逐层关闭                    已改  有                       详情/抽屉/弹层   部分通过
M05   抽屉内弹窗不被误关              已改  有（行为+标记两层）       未执行          部分通过
M06   请求取消                        已改  有                       未执行          部分通过
M07   查询参数保留                    已改  有                       未执行          部分通过
M08   MIME 嗅探                       已改  有（JVM 纯函数测试）      未执行          部分通过
M09   系统文件选择器                  已改  无（框架回调）            未执行          部分通过
M10   前台服务 onTimeout              已改  无（框架回调）            未执行          部分通过
R01   溢出菜单可滚动                  已改  有（2 条）                仅验菜单可打开   部分通过
R02   底栏返回目标列表                已改  有（2 条）                未执行          部分通过
F01   旧文章状态回传                  已改  有（+2 反向对照）         不适用          通过
F02   取消登录不复活                  已改  有（2 条）                不适用          部分通过
F03   映射批次轮转                    已改  有（+反向对照）           不适用          部分通过
F04   同发布时间分页                  已改  有（+反向对照）           不适用          部分通过
F05   迟到账户查询                    已改  有                       不适用          部分通过

附加修复（不属于上述 17 项，设备验证时发现）：
  工具栏溢出弹层未注册返回处理，导致返回键穿透、文章被关闭。已按 BACK_PRIORITY.popover
  注册并补用例与反向对照；设备复验待完整的 Wails 前端构建后再做。

剩余缺口（按优先级）：
  1) 设备复验：M03/M04 余下层级（底部弹层、对话框、查找栏、图片查看器、媒体弹窗），以及
     第 18 轮修复的弹层返回行为。
  2) 受控场景才能验的项：M01（父文档访问）、M07（带查询参数资源）、M08（MIME 嗅探）、
     M09（系统文件选择器回传）、M10（前台服务超时）。
  3) 环境缺失：M02 需要 API 23 及以下镜像，本机只有 API 37。
  4) 无自动化回归：M02/M09/M10 依赖框架回调，当前只在代码层保证。
  5) 真实服务：F01/F02/F03/F04 均未接真实 Folo 账号。
```

### 收尾核对（第 20 轮）

```text
尝试：为让第 18 轮的弹层修复进入安装包，先触碰 main.go 强制 Go 重建，再跑 :app:assembleDebug。
结果：Gradle 1 秒返回 BUILD SUCCESSFUL，APK 时间戳仍为 12:38:21（早于本轮前端产物 13:23），
      即没有重新编译 Go 产物、也没有重新嵌入前端资源。
结论：设备上的安装包不包含第 18 轮修复，因此不进行设备复验，也不以任何重装结果作为证据。
      要做设备复验，必须走完整的 Wails Android 构建（重新生成 Go 侧的 android 产物），
      仅调用 :app:assembleDebug 不够。
说明：build/android/app/src/main/assets 不存在，页面资源是 Go 侧 embed 进二进制的，
      所以 Gradle 的增量判定看不到前端变化。
```

### 复验修复记录（审计 7 项）

```text
M01 — 已修（JVM 回归，设备未复验）
  根因：addJavascriptInterface 注入 WebView 的每一帧，且平台无法限定作用域，文章 iframe 与主文档
  共享同一个原生 API。
  修复：新增 BridgeGuard，用 WebViewCompat.addDocumentStartJavaScript（androidx.webkit 1.9.0）
  在文档开始时运行脚本：顶层文档保留桥接，其余帧先 delete 再显式置 undefined（原生属性可能
  拒绝删除）。限定 origin 为 http://wails.localhost；特性不支持时记录告警而非默认"已生效"。
  回归：BridgeGuardTest 4 项通过（顶部保留、其他帧移除、先删后清、origin 常量）。
  未完成：设备复验；APK 需完整 Wails 构建才能包含改动。

M04 — 已修（守卫测试，设备未复验）
  根因：图库查看器自身既无返回处理也无 Escape，事件落到 App 的范围重置，直接离开图库。
  修复：在拥有 showImageViewer 的父组件注册 useAndroidBack(…, BACK_PRIORITY.viewer)，先关查看器
  再让位。回归：galleryBack.test.ts 2 项（注册存在且优先级为 viewer）；删除注册的对照失败。

M06 — 已修
  根因：体读取是异步的，abort 落在 await 期间时既不会触发 raceAbort 的监听，也没有二次检查，
  原生调用照常发出并返回 200。
  修复：读 body 后复查 signal.aborted；raceAbort 入口处理"已 abort"信号（补发 Cancel）。
  回归：新增「POST 在读 body 期间被取消不得发出原生调用」；去掉复查的对照失败。

F01 — 已修（两个独立 bug）
  根因一：复查窗口以本地文章时间为中心，而匹配按 URL，两端发布时间可不同，±1ms 必然打空。
  修复一：映射表新增 published_at，记录远端条目自身时间；旧数据回退到本地时间并用 ±24h 窗口。
  根因二：取满一页（8 条）后水位停在最后一条，下一页查空直接返回，重查永久停止。
  修复二：查空且水位非 0 时回绕到 0。
  回归：两用例（远端时间不同仍可回传、水位末尾回绕）；两个对照各自失败。

F03 — 已修
  根因：无变化的映射行直接 continue，不写 updated_at，最旧优先的批次永远轮到同一批，后面的
  待推送修改永远进不了窗口。
  修复：无变化行也调用 UpdateFoloSnapshot 以轮转。
  回归：501 条映射、仅最后一条待推送，三次同步内必须推送；去掉打戳的对照失败。

F04 — 已修（后端判定 + 前端提示）
  根因：只有"整页结束且下一页为空"才算截断；同时间边界下还有更旧条目时下一页非空，于是漏报。
  修复：整页结束于重复时间戳即判定"无法证明完整"，记录该时间带；前端在 truncated 时追加警告
  文案而非只报成功。回归：101 条同一时间 + 5 条更旧，断言 truncated 且时间带正确；恢复旧判据的
  对照失败。
  已知取舍：该判据偏保守，时间带恰好整页装下时会误报；截断标记持久，缺口不消失就一直提示。

M08 — 未修
  根因确认：上游 Content-Type 只存在于 Go 的资源代理（media.HandleWebpageResource）中，Java 的
  serveApiSubresource 只拿到字节，无扩展名时只能回退 text/html。
  修复路径：由 Go 在重写子资源 URL 时带上上游 content-type（如 &ct=text%2Fcss），Java 读取该参数
  并优先采用；需同时补 Java 侧解析测试与 Go 侧断言。
```

### 执行记录：审查修复 + 设备复验（第 21 轮）

```text
M01 — 已修并取得设备证据
  审查发现（自查）：上一版把 document-start 脚本的 allowedOriginRules 限定为 http://wails.localhost，
  而文章 iframe 是 sandbox 无 allow-same-origin，源为不透明来源——脚本根本不会注入到需要它的帧里，
  等于没修。改为通配 "*"，由脚本里的 window.top===window 判断保留顶层、其余帧移除。
  设备复验：Pixel_10 / API 37，重装 app-debug.apk（14:31:42），CDP 探针
  docs/evidence/android-bridge-probe.cjs 输出：
    top_frame="object"        顶层文档保留桥接
    sandbox_frame="wails=undefined"  沙箱不透明源帧内已移除
  回归：BridgeGuardTest 4 项（顶部保留、其余帧移除、先删后清、通配规则）。
  未完成：未做“把规则改回应用 origin”的对照（需要再两次构建+安装），因此“通配是生效前提”这一点
  依据的是文档语义与探针结果，不是对照实验。

ImageViewerModal — 已修（审查第二遍发现）
  问题：查看器自身既无返回处理也无 Escape，只能点背板退出。
  修复：注册 keydown Escape → emit('close')，onUnmounted 移除监听。
  回归：galleryBack.test.ts 增加 1 项（Escape 判定、emit close、卸载清理），共 3 项通过。

仍未修复（本轮列入未完成，不冒充完成）：
  - F04 判据偏保守：整页结束于重复时间带即标记截断，时间带恰好整页装下时会误报；改进方向是
    对时间带做一次探针查询，只有在“上游返回的该时间带条数多于已收集条数”时才标记。
  - 截断标记持久：缺口无法恢复时会一直提示，尚未设计“何时清除”。
  - M04 设备复验：前端改动需要完整 Wails Android 构建重新嵌入 Go 侧资源；:app:assembleDebug
    不会重编 Go 产物，因此本轮未在设备上验证图库返回与弹层返回。

CI（与验收无关，同期改动）：
  - android.yml 的 upload-artifact 增加 retention-days: 7。
  - release.yml 支持“版本留空 = 滚动 latest”：版本取自 frontend/package.json，不建 tag，
    发布为 latest 预发布；滚动模式下跳过产物存在性检查以强制重建。
  - release.yml 在发布后清理 latest 中非当前版本名的旧资产，避免文件堆积。
```

### 执行记录：手机端插件裁剪 + 完整构建（第 22 轮）

```text
构建链路（本轮第一次真正跑通完整构建）
  Task 的 android 任务假定 Unix shell（uname/export/tail），在本机 Windows 下失败：
    "tail": executable not found; Unsupported architecture: arm64
  改为在 PowerShell 里执行等价步骤：
    CC/CXX = NDK 29.0.14206865 的 x86_64-linux-android21-clang(.cmd)
    CGO_ENABLED=1 GOOS=android GOARCH=amd64
    go build -buildmode=c-shared -tags android,debug -buildvcs=false -gcflags=all="-l" \
      -o build/android/app/src/main/jniLibs/x86_64/libwails.so        （49s，36MB，14:39:37）
  再 :app:assembleDebug → app-debug.apk（47MB，14:39:52，含新前端与桥接守卫）。
  这一步是此前“APK 陈旧”的根因：assembleDebug 不会重编 Go 侧，必须显式编 libwails.so。

手机端插件裁剪 — 设备验证通过
  设备：Pixel_10 / API 37，phoneWidth=411（手机断点生效）。
  探针：docs/evidence/android-plugin-visibility.cjs —— 点开设置→插件，读取真实 DOM 文本。
  结果：底部文本为 “Reader / Miniflux 集成 / Folo 集成 / RSSHub 集成”，
        Notion=false Obsidian=false SiYuan=false Zotero=false，
        FreshRSS=true Folo=true RSSHub=true。
  即：手机端只保留 RSS 相关插件，四个笔记/知识库集成不再渲染。

仍未完成
  - 文章工具栏弹层的返回复验：本轮用 CDP 点击“更多”未能让菜单出现（选择器命中的可能是列表头的更
    多按钮），因此没有取得该弹层的设备证据；探针脚本已删除，未留下半成品。
  - F04 判据偏保守 + 截断标记持久（此前已记录）。
  - M08 无扩展名 CSS（此前已记录）。
```

### 执行记录：F04 判据修正 + 弹层返回设备复验（第 23 轮）

```text
F04 — 由“只能如实报告缺口”升级为“先补取，只有真装不下才报告”
  问题：整页结束于重复时间带时一律标记截断，时间带其实能被枚举时属于误报，而且被页边界切掉的
  条目从此再也不会被取到（游标已越过该时间带）。
  修复：在走路结束时对时间带单独发一次查询（Limit=syncPageSize，NewerThan=T-1ms，
  OlderThan=T+1ms），把未见过的条目补进来；只有该时间带自身就填满一页（返回条数 >= 页大小）时
  才标记 truncated。这样误报消失，且被切掉的条目真的被取回。
  回归：
    - TestSyncRecoversTheBandWhenItCanBeEnumerated（97 条较新 + 5 条同一时间）：期望处理 104 条
      （100 + 2 + 补取 2）且不标记截断；去掉补取的对照得到 100，用例失败。
    - TestSyncReportsTruncationWhenOlderEntriesExistBelowTheBand 仍通过（该带 101 条 > 页大小）。
    - TestSyncReportsTruncatedCoverageForOnePublishTimeOverflow 的请求次数断言由 3 改为 4，
      并额外断言第三次调用是“两侧都有限定”的时间带查询——这是本次契约变化，不是放宽。
  结果：go test ./internal/... 全通过，go vet 干净。

文章工具栏弹层返回 — 设备验证通过（此前两轮都缺这条）
  设备：Pixel_10 / API 37，新包 14:39:52。
  探针：docs/evidence/android-popover-back.cjs —— 精确点击 aria-label 为「更多操作」的按钮
  （此前误点了列表头的「更多」，所以一直没打开），再触发系统返回键。
  结果：菜单打开（新增文本：从稍后阅读中移除 / 在浏览器中打开 / 复制标题 / 复制链接 /
  重新加载文章内容）；返回后菜单文本消失（menuStillThere=false），文章详情仍在
  （detailStillOpen=true）。即返回只关闭弹层，没有退出文章。

仍未修复
  M08 — 无扩展名 CSS：上游 Content-Type 只存在于 Go 的资源代理返回里，Java 的 serveAsset 只拿到
  字节。可行修法是在服务端重写子资源 URL 时按引用标签附带类型提示（link→text/css、script→js），
  Java 优先读取该参数。这需要同时改 media_proxy.go 的 4 处构造点、MainActivity 的 sniffMimeType，
  并各补测试；本轮剩余的验证预算不足以完整落地，遂停手而不是留下半改状态。
```

### 执行记录：M08 修复（第 24 轮）

```text
M08 — 已修（单元证据，未在设备上复验）
  根因（已确认）：上游 Content-Type 只存在于 Go 资源代理的响应里，桥接的 serveAsset 只把字节交给
  Java，无扩展名的合法 CSS 因此只能回退 text/html。
  修复（跨层，两侧都改）：
    Go  media_proxy.go：在能确定类型的重写点给代理 URL 追加 &ct=<type>
        - 样式表引用：重写 <link> 的 href 时，若该标签含 rel=stylesheet → ct=text/css
        - CSS 内部：rewriteCSSURLs 的两处 @import 重写 → ct=text/css
    Java MainActivity.sniffMimeType：优先读取 ct 参数（经 plainQueryParam 做百分号解码；
        base64 参数用的 queryParam 会把值解坏，故单独加了一个读取器），
        ResourceMime.fromTypeHint 负责校验形状（必须是 type/subtype，否则忽略）。
  回归：
    - TestRewriteKeepsStylesheetTypeHint / TestRewriteKeepsCSSImportTypeHint：断言重写结果含
      ct=text%2Fcss；分别移除对应追加的对照均失败。
    - ResourceMimeTest 新增 2 项（合法提示被接受；null/空/纯文本/注入内容被拒绝），现共 6 项通过。
  未完成：设备复验。当前模拟器里的 APK（14:39:52）早于本轮 Go/Java 改动，需要再次
  编 libwails.so + assembleDebug 才能含 M08；本轮未重跑该链路，故不声称设备通过。
```

### 执行记录：M08 设备复验（第 25 轮）

```text
重新构建：libwails.so 14:57:57 → app-debug.apk 14:58:00 → 安装并启动（pid 30379）。

M08 — 设备端到端验证通过
  做法：宿主机起一个 fixture（docs/evidence/m08-fixture-server.cjs，端口 18777），提供
    /page      含 <link rel="stylesheet" href="/bare-css"> 的 HTML
    /bare-css  无扩展名，响应头 Content-Type: text/css，规则为 #probe{background:rgb(1,2,3)}
  探针 docs/evidence/android-m08-stylesheet.cjs：
    1) 让应用内的 Go 代理抓取该页面（/api/webpage/proxy?url=...），读回重写后的 HTML；
    2) 用 iframe 加载代理后的页面，等 2.5s 后读 styleSheets 与该元素的 computedStyle。
  结果：proxiedStatus=200、rewritten=true（HTML 中出现 ct=text%2Fcss）、sheets=1、
        background="rgb(1, 2, 3)"。
  判定：样式规则只有在 MIME 被判定为 text/css 时才会应用；背景色生效即证明无扩展名的 CSS
  不再回退成 text/html。这是 M08 的验收点。

仍未验证（唯一剩余项）
  图库返回的设备复验：需要一篇含图片的文章才能进入图库视图。当前模拟器里是 fixture 订阅源，
  文章只有文字，没有可进入图库的入口，因此本轮没有取得该层证据；代码层有 galleryBack 守卫测试。
```

### 执行记录：图库返回的 fixture 与部分验证（第 26 轮）

```text
fixture：新增 docs/evidence/gallery-fixture-server.cjs（端口 18778）
  /feed.xml  一条 item，description 内含两张 <img>
  /img/*.png 真实 PNG 字节
  通过应用 API 添加：POST /api/feeds/add {url}（注意不是 POST /api/feeds —— 后者只返回列表，
  且会给出 200，容易误判为添加成功）。结果：feed id=3，article id=13「Gallery fixture post」。

设备观察（Pixel_10 / API 37，包 14:58:00）
  - 打开该文章：detailOpen=true，页面内 3 张图片 naturalWidth>0（alpha.png ×2、alpha-b.png），
    说明带图文章已经能进应用。
  - 点「多媒体模式」进入图库：12 张图片渲染，视图标题为「多媒体模式」。
  - 在图库中点击第一张图片：探针未观察到查看器（无「缩略图/上一张/下一张」类标签），
    因此"查看器层的返回"仍未取得证据。
  - 从图库按返回键：仍停留在图库（12 张图片、标题不变），没有退出到文章列表。

结论：M04 的图库层级"返回不会把人踢出图库"这一点有了正面观察；但查看器层的返回仍未被观测到，
因为探针没能打开查看器（点图片元素未触发）。如实记为"部分验证"。
```
