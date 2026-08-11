# httpYac ↔ jq-view 集成 (方案 B + C)

把 httpYac 的响应实时导流到 jq-view 可视化。发请求 → jq-view 网页自动刷新成最新响应 → 在 jq 输入框用 `todate` / `strftime` 就地转时间戳。

## 一次性准备

1. 装 jq-view 进 PATH(在本仓库根目录):
   ```bash
   go install .
   ```
2. 把 `.httpyac.js` 拷到你**实际用 httpYac 的项目根目录**。

## 每次使用:只管点 Send

在 VS Code 里正常点发送 `.http` 请求即可。hook 全自动处理:

- jq-view 没起 → 自动后台拉起(常驻,不随 httpYac 退出)+ 自动开一次浏览器。
- 已经起着 → 直接推数据,复用同一个浏览器 tab,SSE 实时刷新,不重复开 tab。

然后在 jq 输入框转时间戳,例如:
- `.[].created |= todate` → ISO 日期
- `.[].created |= strftime("%Y-%m-%d %H:%M:%S")` → 自定义格式

> 手动起服务(可选):`./start-jqview.sh` 不弹浏览器 / `./start-jqview.sh 8080 open` 弹浏览器。

## 原理

- **C(jq-view 侧,已实现)**:新增 `POST /api/push`(接收 JSON)、`GET /api/current`(取最新),并把 `/api/reload` 的 SSE 升级成命名事件 `reload`(重启整刷)+ `data`(推送时原地换数据,保留当前 jq 表达式)。
- **B(httpYac 侧,`.httpyac.js`)**:全局 `onResponse` hook,把每个 JSON 响应 POST 给 `/api/push`;jq-view 没起时降级为 `spawn('jq-view', ['-open'])` 临时实例。hook 不阻断 httpYac 自身的响应展示。

## 注意

- 非 JSON 响应(HTML/二进制)会被 hook 按 content-type 过滤掉,不推。
- 端口要一致:`.httpyac.js` 里 `JQVIEW_URL`(默认 `http://localhost:8080`)与 `start-jqview.sh` 的端口。可用环境变量 `JQVIEW_URL` 覆盖。
- httpYac 脚本沙箱是否放行 `require('child_process')` / `fetch` 取决于其版本;若 hook 不生效,先单独用 `.http` 响应脚本里的 `spawn` 试链路。
