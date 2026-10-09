# 本次实施状态与验收

日期：2026-10-08。独立分支 `codex/wslpp-rearchitecture`。用户授权先单独提交计划、首先搭建简单 UI，之后谨慎推进核心重构。

## 已实现

| 领域 | 当前交付 |
| --- | --- |
| UI | Vue 3 / TypeScript / Pinia / Naive UI / Vite PWA；概览、端口详情、JSON 配置、最近日志；接真实 API |
| 配置 | 兼容旧 JSON/块注释；映射、协议 ACL、重复/未知字段校验；revision 与已观察的外部编辑冲突；首次覆盖备份；保留最后有效配置 |
| TCP | 双向半关闭、取消拨号、活跃连接 Stop/重启、端点限额、ACL 热更新、统计 |
| UDP | 显式 opt-in；每客户端 session、空/大报文、多回复、稳定源端口、TTL、有界队列、停止回收 |
| WSL | 默认发行版重新发现、UTF-8/UTF-16、显式发行版命令、超时、`ip -j` / `ss` 解析、NAT/mirrored 识别、stopped/unknown 区分 |
| 协调 | config + snapshot 纯规划、协议端点身份、未变化资源保留、失败退避、15 秒 stale 宽限、管理端口保留 |
| App/API | 单一装配根、数据目录锁、日志轮转、真实运行状态、本机 Host/Origin/peer 校验、一次性授权与会话 |
| CLI/SCM 适配 | run/status/ui/config/doctor、账户/SID 跨 UAC 保留、SCM 生命周期、服务登录权利、固定 exe/数据目录、DACL、升级健康失败回滚 |
| 构建 | UI 嵌入单个 Windows exe；amd64/arm64 构建与校验文件；Windows/Linux CI 工作流 |

SCM 适配已实现，不代表“未登录时 WSL 一定可运行”已经成立。参考项目的用户服务/自动启动方式没有直接套到 LocalSystem：发行版所属用户是本项目必须单独验证的边界。

## 2026-10-09 Windows 实机调整

在 Windows 11 26100 / WSL 2.4.11 / Ubuntu（NAT，localhostForwarding=false）上前台运行验证：

- 修复网络模式探测：原代码固定执行 `/usr/lib/wsl/wslinfo`，该版本实际位于 `/usr/bin/wslinfo`（指向 `/init`），导致所有扫描失败。现在在发行版内用一次 `sh -c` 合并探测 wslinfo / `ip -j` / `ss`，wsl.exe 调用从每轮 6 次降到 3 次，轮询间隔 1s → 2s（配置变更仍立即唤醒）；移除 `--legacy-nat`。
- 新增配置 `distro`：省略时跟随 WSL 默认发行版，可固定其他发行版；`doctor --distro` 输出发行版列表与发现的端口。
- 按用户要求移除管理 UI/API 的 token、一次性链接与会话；保留 loopback 监听与 peer/Host/Origin 校验。
- 实测：TCP（loopback 与局域网 IP）、UDP 回显、API 写配置、跨站 Origin/伪造 Host 拒绝、WSL 端口关闭后代理在一轮扫描内移除并释放 Windows 端口。
- 构建对齐 ssh-tunnel-service：pnpm + ESLint、`internal/version`、Makefile 目标、GoReleaser 与 tag 发布工作流。

服务模式（SCM 安装、未登录启动）仍未实测。

## 当前验证范围

本次在 Linux 环境执行：Go 包测试/race/vet、真实本机 TCP/UDP socket 回归、API/配置/控制器测试、升级失败事务测试；Vue 类型/发布构建、npm audit，以及 Chromium 授权/配置/诊断/移动宽度/服务断线禁用保存检查。Windows amd64/arm64 进行交叉编译，Windows 代码进行 vet。

CI 工作流已经加入，但当前没有推送或执行 GitHub Actions；不能称 Windows 原生 CI 已通过。安装失败的清理保留导入配置与数据；已经授予的服务登录权利不自动撤销，避免误删原有授权。

当前没有 Windows SCM 安装、账户凭据、Session 0、DACL 效果、WSL 实际转发结果。

## 与调查计划的差异

- UI 按用户最新要求先搭骨架，再接入核心。
- 采用标准库内核；GOST 的同测试运行时比较未执行，依据与限制见 [ADR](adr-001-proxy-kernel.md)。
- 实机环境缺失，先实现可隔离的 SCM 适配并补 Windows 验收清单；生产安装/发布仍不能跳过服务身份可行性门槛。
- 目标/发行版/资源参数改变采取关闭后重建；listener 原子换目标、普通配置变更的 TCP 排空尚未实现。
- 尚未提供规则暂停、每 IP/全局配额、UI 表单规则编辑、完整中英切换、PWA 更新提示或 API 版本协商。这些不属于已完成能力。
- 原端口探测测试中“占一个端口再释放来模拟不可达目标”仍有小范围 OS 竞态；代理监听本身使用端口 0。未引入完整网络仿真框架。

## Windows 实机发布门槛

先记录 Windows build、WSL 来源与版本（Store/内置）、发行版、账户类型、网络模式；每项保存命令输出与服务日志，不能只记录“可以启动”。

1. 在 WSL 所属账户执行 `doctor`，核对 SID、默认发行版、NAT 与真实端口；验证旧配置导入和 UDP 默认关闭。
2. `install` 后核对 SCM Log On 账户、登录权利、延迟自动启动/恢复配置；密码错误应可诊断且不留下无法管理的半安装服务。再次 install 不覆盖现有 exe/配置。
3. 检查 `%ProgramData%\wslpp` ACL：所属账户被显式授予 exe 读执行及数据写权限，exe 写权限仅授予 Administrators/SYSTEM；其他普通账户不能读取配置（管理 API 无登录，依赖 loopback + Host/Origin 校验）。验证临时替换后的新文件继承正确权限。
4. 冷启动且完全未登录，通过另一台设备/管理员远程查询 SCM 状态；WSL 停止时服务可运行并等待。按现行策略，服务不会主动启动发行版，不能把 waiting 解释为失败。
5. 在没有该用户交互登录的条件下，由授权方式启动该账户的 WSL 工作负载，验证 SCM 进程能看到所属发行版并转发 TCP/UDP。若 Session 0/profile 不可用，先修改账户执行架构；不能改为依赖登录会话 helper 规避要求。
6. 检查注销、WSL 停止/重启、默认发行版改变、IP 改变、睡眠/恢复、命令超时；监听资源释放、unknown/stale 与重试行为符合预期。
7. TCP 验证 SSH 长连接、半关闭后响应、Stop 活跃流量；UDP 验证 DNS/实际目标负载、双客户端、异步回复、空/大报文、TTL、不可达目标与 ICMP。
8. 分别验证具体监听地址/wildcard、多网卡、localhostForwarding、防火墙、端口排除范围与冲突解除后恢复；检查 UDP 回复的来源地址。
9. mirrored 下只报告网络模式并停止自有代理，确认没有同端口回连自身；没有 wslinfo 的旧 WSL 按 NAT 处理（mirrored 需要带 wslinfo 的新版 WSL）。
10. 执行 stop/restart/update/uninstall；退出后端口可立即重用，卸载保留数据。注入备份/替换/启动/健康检查失败，验证错误明确、旧 exe 可恢复，回滚失败保留备份。
11. 改变服务账户密码、撤销登录权利、使用 Microsoft/域账户或域策略，核实可诊断性与修复步骤。
12. 实机 UI 验证一次性链接、401/会话失效、非法 Origin/Host、配置 revision 冲突、服务停止后禁用提交、重连、PWA 安装及升级缓存。

若要让机器冷启动后连 WSL 工作负载也自动启动，需要另行明确 opt-in 启动策略；当前保留旧工具“不主动唤醒 WSL”的约定。
