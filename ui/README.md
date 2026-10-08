# wslpp UI

Vue 3 + TypeScript + Pinia + Naive UI + Vite PWA 的初始管理界面。API 类型和访问在 `src/api.ts`，运行状态在 `src/store.ts`，页面暂集中在 `src/App.vue`；后续 UI 框架调整无需改变后端协议。

```bash
cd ui
npm ci
npm run dev
npm run build
```

开发服务只绑定本机。`/api` 默认转发到 `http://127.0.0.1:47831`。构建输出至 `internal/web/static`，由 Go 发布构建嵌入。未连接后端时显示不可达状态，不使用演示数据。

概览、端口列表、JSON 配置编辑与最近诊断先提供简单实现；具体视觉、组件拆分、表单交互后续独立完善。PWA 仅缓存静态资源，不缓存动态 HTML、API 或授权响应。
