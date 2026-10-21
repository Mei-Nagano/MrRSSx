# Reader integrations

MrRSS connects to external readers and hosted services. Enable and configure each integration under **Settings → Plugins**; any combination may be active at once.

## FreshRSS and Miniflux

Configure each integration separately under **Settings → Plugins**. Both may be enabled at the same time. Each uses its own Google Reader API username and password, server root URL, sync button, and last-sync time. Miniflux must have the Google Reader API integration enabled on its server.

Global refresh syncs each enabled reader. A subscription's context-menu sync and article read/favorite changes go only to the reader that owns it. Identical feed URLs, article URLs, and remote stream IDs from different readers remain separate local records. Disabling an integration removes only that integration's locally synchronized data, after the existing confirmation.

On upgrade, a legacy `freshrss_provider=miniflux` configuration moves to the `miniflux_*` settings. The migration retains encrypted credentials, subscription and article IDs, reading state, favorites, read-later membership, and queued changes. The FreshRSS integration starts disabled with empty credentials. Existing FreshRSS configurations remain in place. The migration is transactional and runs once.

The existing `/api/freshrss/{sync,sync-feed,status}` endpoints address FreshRSS only. Miniflux uses `/api/miniflux/{sync,sync-feed,status}` with the same request and response shapes. The legacy `is_freshrss_source` feed flag continues to mean a remotely managed subscription; `sync_provider` identifies its owning reader. Local feeds have the flag set to false.

## Folo

Enable **Folo** under **Settings → Plugins** and sign in through the browser prompt. A headless server can paste a one-time or session token instead. The session token is stored encrypted and is only sent to the configured Folo API.

**Import subscriptions** adds the account's Folo subscriptions as ordinary local feeds, grouped by Folo folder (`category`) or by Folo view type (`view`); MrRSS then refreshes them itself.

Read and favorite state are reconciled only when **Sync now** is pressed; there is no background sync. Entries are matched to local articles by URL. When both sides changed since the last sync, the local state wins, and a change that fails to reach Folo stays pending for the next attempt. Signing out clears the stored session and the match records but keeps imported feeds and articles.

## 中文说明

在**设置 → 插件**中分别配置 FreshRSS 和 Miniflux，可同时启用。两者各自保存服务器地址、Google Reader API 用户名及密码，并提供独立的同步按钮和上次同步时间。Miniflux 服务端需要先启用 Google Reader API 集成。

全局刷新会同步所有已启用的集成。单个订阅的同步、文章的已读和收藏状态仅回传至所属服务。同名、同 URL 或相同远端 ID 的订阅及文章不会跨服务合并；关闭其中一个集成只清理它自己的本地同步数据。

旧版通过 FreshRSS 面板配置的 Miniflux 会自动迁移到独立设置，并保留加密密码、订阅、文章、阅读状态、收藏、稍后阅读和待同步队列。迁移后 FreshRSS 默认关闭，原有 FreshRSS 用户的配置不变。

### Folo

在**设置 → 插件**中启用 Folo，并按提示在浏览器里登录；无法打开浏览器的服务器可以粘贴一次性令牌或会话令牌。令牌加密保存，只会发送到配置的 Folo API。

**导入订阅**会把账号中的 Folo 订阅添加为普通本地订阅源，可按 Folo 文件夹（`category`）或按 Folo 视图类型（`view`）分组，之后由 MrRSS 自行刷新。

已读和收藏状态只在点击**立即同步**时同步，没有后台定时同步。条目按 URL 匹配到本地文章；双方自上次同步后都发生变化时以本地为准，未能送达 Folo 的修改会保留到下一次重试。退出登录会清除令牌和匹配记录，但保留已导入的订阅和文章。
