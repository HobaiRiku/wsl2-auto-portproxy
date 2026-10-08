# wslpp

Windows 上的 WSL2 TCP/UDP 端口转发工具。扫描默认 WSL2 发行版在 wildcard 地址上监听的端口，将 Windows 监听端口转发到 WSL 的 NAT 地址；支持映射、忽略和按协议设置访问规则。

当前重构提供简洁的 Vue 管理界面、本机 API 和 Windows SCM 安装入口。**未登录账户的 WSL/Session 0 行为仍须按目标 Windows/WSL 版本实机验收**，现有交叉编译结果不代表该场景已验证。详见 [实施状态与验收](docs/design/implementation-status.md)。

## 范围

- 默认 WSL2 发行版、NAT；依赖发行版中的 `iproute2`（`ip` / `ss`）。
- 识别实际 mirrored / 其他网络模式，显示状态并停止自身转发，避免回连自身。
- TCP 默认启用；UDP 需显式设置 `udpEnabled: true`。UDP 支持固定目标的 unicast 报文，客户端 session 独立，不支持广播、组播和透明源地址。
- WSL 已停止时只查询状态并等待；服务自动启动不代表自动启动 WSL。状态查询与随后执行 Linux 命令之间仍有 OS 级竞态，无法承诺原子“不唤醒”。
- 有 `wslinfo --networking-mode` 的 WSL 版本自动识别模式；旧 WSL 无此能力时停止转发。确认旧版本处于 NAT 后可显式使用 `--legacy-nat`。

## 构建与开发

需要 Go 1.24+、Node.js 22+、npm。Windows 原生 race 测试还需要 C 编译器。

```bash
make build                         # dist/wslpp.exe，嵌入 UI
make release                       # Windows amd64/arm64 + SHA256SUMS
make test
make race
make vet
```

开发模式开启两个终端：

```bash
make dev                           # --home .wslpp-dev --ui-dev
cd ui && npm ci && npm run dev      # http://127.0.0.1:5173
```

在 Windows 上执行 `wslpp ui --home .wslpp-dev --ui-dev` 建立授权。Linux 可运行核心/API 测试和 UI 开发，但没有 Windows WSL 发现或服务管理能力。直接 `go build .` 不嵌入页面；发布构建需先构建 UI，再加 `-tags embedui`。

## 运行与管理

```powershell
.\wslpp.exe run
.\wslpp.exe ui
.\wslpp.exe status
.\wslpp.exe doctor
.\wslpp.exe config get
.\wslpp.exe config set .\config.json
```

无参数启动与原来的前台方式兼容；`-v` / `version` 输出版本。前台默认数据目录为 `%USERPROFILE%\.wslpp`，可由 `--home` 或 `WSLPP_HOME` 指定。API 默认 `127.0.0.1:47831`；可用 `--listen` 指定其他 loopback 端口。

`ui` 通过受目录权限保护的本机 token 申请一分钟有效的一次性链接，浏览器建立 HttpOnly 会话。复制该链接也能在同一台机器上打开；失效后重新运行 `ui`。PWA 只缓存静态资源，API、授权和 HTML 不缓存；服务不可达时禁用保存。

UI 当前只实现概览、端口表格、JSON 配置和最近日志，后续视觉与框架调整可以独立进行。界面中 active 表示监听成功，不表示后端应用健康。

## Windows 系统服务

服务必须使用**拥有该 WSL 发行版的 Windows 账户**运行；不默认使用 LocalSystem。系统级 SCM 注册与运行账户是不同的概念。

```powershell
.\wslpp.exe install                 # UAC 提升；保留提升前的账户/SID
.\wslpp.exe start
.\wslpp.exe status
.\wslpp.exe ui
.\wslpp.exe stop
.\wslpp.exe restart
.\wslpp.exe uninstall               # 保留配置、token、日志
```

安装时在提升后的控制台输入 Windows 账户密码（不是 Hello PIN）；密码不写入参数、配置或日志。显式账户可使用 `install --account 'COMPUTER\user'`。本地/Microsoft/域账户兼容性、profile 与 Session 0 可见性均须实测；密码或域策略变更后需要更新 SCM 登录凭据。

固定 exe：`%ProgramData%\wslpp\bin\wslpp.exe`。数据：`%ProgramData%\wslpp\data`。二进制写权限授予 Administrators/SYSTEM，所属账户另被授予读执行权限；数据目录允许所属账户、Administrators、SYSTEM。安装时可导入原有配置，但不覆盖已有服务配置。`install --home` 只用于定位导入源，服务数据目录仍固定。CLI 优先发现用户目录中的前台实例，然后查找服务目录；可显式指定 `--home`。

安装只注册服务，不启动；SCM 配置为延迟自动启动并在崩溃后恢复。从新版本所在的其他目录执行 `wslpp.exe update`：停止并等待、备份旧 exe、替换、启动与管理 API 健康检查，失败恢复旧 exe。此流程也必须通过 Windows 实机故障验收；健康检查只证明进程/API 可达，不能证明 WSL 转发可用。

## 配置

`config.json` 沿用原 JSON，兼容旧的块注释。UI/API 保存时标准化，并在首次覆盖已有文件前保留 `config.json.bak`。

```json
{
  "schemaVersion": 1,
  "onlyPredefined": true,
  "listenAddress": "0.0.0.0",
  "predefined": { "tcp": ["666:22"], "udp": ["5353:53"] },
  "ignore": { "tcp": [445], "udp": [] },
  "allowlist": {
    "tcp": { "666": ["192.168.1.0/24", "10.0.0.5"] },
    "udp": { "5353": ["192.168.1.0/24"] }
  },
  "udpEnabled": true,
  "maxConnections": 128,
  "udpIdleSeconds": 60
}
```

- `666:22`：Windows 666 → WSL 22；TCP/UDP 同数字端口可独立使用。显式映射优先于自动扫描的同号端口。
- `onlyPredefined`：只转发定义且已在 WSL 被发现的端口；默认 false。
- `ignore`：按 WSL 远端端口忽略，优先于显式映射。
- `allowlist`：按 **Windows 本地端口和协议** 设置。未配置的端口不受限制；空数组只允许 loopback；loopback 始终允许。收紧规则会关闭不再允许的活跃会话。
- `listenAddress`：转发的绑定 IP；默认 `0.0.0.0`。管理 API 始终只允许 loopback。
- `maxConnections`：每个监听端点的 TCP 连接或 UDP session 上限，0/未设置使用 128，最大 1024；不是全局或每 IP 配额。
- `udpIdleSeconds`：UDP 双向空闲回收时间，0/未设置使用 60，最大 86400；session 的发送队列固定 8 个报文。

首次配置无效时不启动代理，UI 仍可修复。之后的无效编辑/删除保留最后有效配置并报告错误；API 保存带 revision，并检测已观察的外部文件变更。未知字段、重复字段、无效映射和重复本地端口均被拒绝。

未变化的路由保留监听器。目标地址、发行版或资源参数改变时关闭连接并重建；尚未实现旧连接排空。端口占用显示 blocked，并以 1–30 秒退避重试。发现错误显示 unknown/stale，保留上次快照最多 15 秒，然后关闭旧目标代理。WSL 明确停止则立即关闭。

Windows 防火墙、WSL localhostForwarding 和端口排除范围可能影响实际绑定/访问，需要按目标机器验证；wslpp 不自动修改防火墙。后端看到代理源地址，不保留客户端源 IP。

## 设计与验收

- [原始调研与计划](docs/design/wslpp-rearchitecture-plan.md)
- [代理内核选型](docs/design/adr-001-proxy-kernel.md)
- [实施状态与 Windows 验收](docs/design/implementation-status.md)

MIT License.
