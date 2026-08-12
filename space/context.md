# jq-view 工作进度

## 2026-08-11

### 已完成

**1. httpYac ↔ jq-view 集成(方案 B + C,全自动)**

目标:httpYac 发请求 → 响应自动导流到 jq-view 可视化 → jq 里转时间戳。

- `internal/web/handler.go`:新增线程安全 `pushHub`。新增 `POST /api/push`(接收 JSON 并广播)、`GET /api/current`(取最新数据+rev)。`/api/reload` 的 SSE 升级成命名事件 `reload`(服务重启→整页刷新)+ `data`(推送→原地换数据)。
- `internal/web/app.js`:SSE 监听改用 `addEventListener`;新增 `applyData`/`fetchCurrent`,收到 data 事件原地换数据、保留当前 jq 表达式。
- 仓库根新增 `.httpyac.js`(拷到实际用 httpYac 的项目根即可)、`start-jqview.sh`、`HTTPYAC.md`。
- `.httpyac.js` 全自动:点 Send → 探活 `/api/push` → 连不上就 `detached` 后台拉起常驻 jq-view(自动开一次浏览器)→ 等就绪 → 推送。同一时刻只允许一次拉起(`starting` 守卫),避免并发请求抢端口。已重启的 tab 靠 SSE 实时刷新,不重复开新 tab。
- 选型过程:曾考虑「VS Code 内嵌 Simple Browser」和「写 VS Code 扩展」两条不切窗路线,用户最终选定「外部浏览器但全自动」,接受单屏仍需切窗但零手动准备的取舍。

**2. UI 增强:侧边栏折叠 + 时间戳快捷转换**

- 侧边栏折叠:`?sidebar=collapsed` URL 参数控制默认展开/收起(`internal/web/app.js` 里 `sidebarCollapsed` ref 读 `URLSearchParams`)。侧边栏顶部常驻一条折叠开关条(`«`/`»`),收起时 grid 从 `420px 1fr` 变 `32px 1fr`。
- 时间戳快捷转换:jq 输入框旁新增 `⏱ Time` 下拉,自动扫描当前数据里疑似时间戳的数字字段(9-10 位=秒、12-13 位=毫秒,`detectTimestampUnit`/`collectTimestampFields`),选中后自动填充 jq 表达式并立即执行。
- 时区:默认转北京时间(UTC+8),格式贴合用户习惯 `2026-08-11 17:11:21`(空格分隔,无 T、无时区后缀)。表达式:
  - 秒:`.<path> |= ((.+8*3600) | strftime("%Y-%m-%d %H:%M:%S"))`
  - 毫秒:`.<path> |= (((./1000)+8*3600) | strftime("%Y-%m-%d %H:%M:%S"))`
  - 已用 gojq 独立验证过输出正确(不是给 UTC 贴假时区标签,是真实加偏移量)。
  - 时区硬编码在 `TZ_OFFSET_HOURS = 8` 常量,以后要支持切换时区就在这基础上加个开关。

### 踩坑记录

- **前端资源是 embed.FS,改完必须 `go build`/`go install` + 杀旧进程重启,浏览器刷新没用**。之前改完侧边栏/时间戳功能后用户反馈"没有任何变化",查到是 `~/go/bin/jq-view` 运行的进程编译时间早于代码改动时间——旧二进制在跑。以后每次改 `internal/web/` 下文件,流程固定:
  1. `go build -o jq-view .`(或 `go install .`)
  2. `pkill -f jq-view` 杀掉旧进程
  3. 重新起一个新进程验证

### 待办 / 可能的后续方向

- README.md 目前只有一行占位(`# jq-view-web`),HTTPYAC.md 的用法说明还没链接进主 README。
- 时区目前写死北京时间,没有界面开关。

**3. httpYac 弹窗改为 VS Code 内置 Simple Browser(2026-08-11 追加,彻底零切窗已实现)**

关键发现:`vscode-httpyac` 是在 **VS Code 扩展宿主进程内**跑 httpYac(不是拉子进程),其 `vscodeHttpyacPlugin.ts` 会把 `io.javascriptProvider.require.vscode = vscode` 挂到全局,所以 `.httpyac.js` 里能直接 `require('vscode')` 拿到真实 VS Code API——不需要写扩展。

- `.httpyac.js` 的 `ensureServer()` 冷启动分支改造:先 `getVSCodeApi()` 试 `require('vscode')`,拿到就不传 `-open`(不弹外部默认浏览器),等服务就绪后改用 `vscode.commands.executeCommand('simpleBrowser.show', JQVIEW_URL, {viewColumn: Beside, preserveFocus: true})` 在右侧弹 VS Code 内置 Simple Browser;拿不到(纯 CLI 场景)才回退原来的 `-open` 外部浏览器。
- 关键约束(已验证):httpYac 侧 `loadFileConfig` 每次 Send 都 `force=true` 重新 `require()` 整个 `.httpyac.js`,模块级变量不跨多次 Send 保留状态。但冷启动分支本来就是靠"`push()` 先失败(服务确实没起)"这个真实网络状态触发,不是内存 flag,天然只在服务未启动时触发一次,不需要额外去重。
- **配套关键一步**:只弹内置浏览器还不够,httpYac 自己也会弹一个响应文件 tab(用户想要的是「用我们的输出替换掉 httpyac 自己的输出」)。在 `<你的项目根>/.vscode/settings.json` 里加 `"httpyac.responseViewMode": "none"`(该配置 `scope: resource`,按项目/文件夹生效,不影响其他项目)。设为 `none` 后 httpYac 官方说明是"only use output console",不再弹自己的响应编辑器 tab,视觉上就只剩右侧的 Simple Browser。
- 排查记录:中途一度以为改动没生效,排查后发现是查看了历史遗留的旧错误日志(`space/httpyac-integration/.httpyac.js` 路径不存在的报错是 16:11 的旧痕迹)。真正验证方式是加临时 `debugLog()` 写 `/tmp/jqview-debug.log`,后确认 `exthost.log` 里 `vscode.simple-browser` 扩展被 `onCommand:simpleBrowser.show` 激活,证明调用链路通了;调试代码已清理干净。
- **待用户实测确认**:重新点 Send 后是否只弹右侧 Simple Browser、不再弹 httpYac 自己的响应 tab。

**4. 修复:每次 Send 都开新弹窗(2026-08-11 追加)**

根因(已查证,读 VS Code 1.116 workbench.desktop.main.js 源码确认):VS Code 1.116 新增了原生
Integrated Browser 命令 `workbench.action.browser.open`。simple-browser 扩展的
`simpleBrowser.show` 检测到这个原生命令存在时会直接转发给它(`w()` 探测 + `u()` 转发,见
extension.js)。但原生命令每次调用默认都用新生成的资源 ID 开一个 editor,**除非显式带上
`reuseUrlFilter` 去匹配一个已经打开的、URL(scheme+authority+path+query)相同的 tab**,不然
每次点 Send 都新开一个。之前 `.httpyac.js` 只传了 URL 字符串,没传 `reuseUrlFilter`,这就是
"每次 Send 都弹新窗口"的根因。

副因:`isSimpleBrowserOpen` 用来判断"面板是否被手动关掉"的检测,扫的是老版 webview 的
viewType `mainThreadWebview-simpleBrowser.view`。但原生 Integrated Browser tab 走的是
`Gl`(`workbench.editorinputs.browser`)这个全新的 editor input 类型,根本不是这个
viewType——扫描永远判定"已关闭",于是这条分支也会触发重新打开,加重了重复开窗。

曾试过的修法(已放弃):`openOrFocusBrowser` 用 `reuseUrlFilter: url` 让命令自己复用已开 tab。
问题:一旦把浏览器 tab 拖进辅助侧边栏(Secondary Side Bar,用户实际用法),reuse 匹配基于
`editorService.editors` 找不到它,依然每次新开 → 没解决。

最终方案(治本,双管):
1. **热路径彻底不开窗**:`.httpyac.js` 每次 Send 都被重新 require,没有跨 Send 内存状态判断
   "tab 是否还开着"。而 tab 一旦开着,SSE(`app.js` 的 `data` 事件 → `fetchCurrent` →
   `applyData` 原地换数据)本就会在每次 push 后自动刷新内容。所以热路径(服务已在跑)只
   `push(body)`,【绝不】再调任何开浏览器命令 —— 这才是"每次 Send 开新 tab"的真正根治。开窗只在
   冷启动分支(`ensureServer`)做一次。
2. **关掉 tab = 停服务,下次 Send 自动重开**:给 Go 侧加 `-idle-exit <dur>` flag
   (`main.go` + `internal/web/handler.go`)。浏览器 tab 持有 `/api/reload` 的 SSE 连接,关 tab
   就断连。`pushHub` 订阅数掉到 0 且超过宽限期(`.httpyac.js` 传 `3s`,吸收整页 reload 的 1s 瞬断
   重连)就调 `onIdle` → `os.Exit(0)`。下次 Send 时 push 失败 → 走冷启动重新拉起 + 重新开 tab。
   守卫:必须先有过至少一个 client 连上(`armed`)才武装计时器,冷启动到浏览器首次连上的空窗期
   不会误退。`-idle-exit 0`(默认)= 关闭该行为,不影响 `cat x.json | jq-view` 等独立用法。

验证:go build/vet/test 全绿;运行时实测三条:①SSE 断开 ~2s 后进程退出;②无 client 连上时不
提前退出(等 4s 仍活);③宽限期内重连能取消退出计时器。已 `go install`,杀掉旧的无 idle-exit 的
8080 常驻进程,下次 Send 冷启动即用新二进制。

排查方式:读 `simple-browser/dist/extension.js` + `workbench.desktop.main.js` 里
`workbench.action.browser.open` 的 `run()`,确认 reuse 匹配基于 `editorService.editors`(拖进辅助
侧边栏后匹配不到),据此放弃 reuse 路线改走"热路径不开窗 + idle-exit"。

**待用户实测确认**:Send 多次只保留一个 tab 原地刷新;手动关掉 tab 后再 Send,会重新弹出。

**5. 修复:"cannot iterate over: null"(2026-08-11 追加)**

现象:jq-view 报错框显示 gojq 的 `cannot iterate over: null`。不是崩溃,是当前 jq 表达式对当前
数据不适用——表达式引用的路径在数据里是 `null`/缺失,却对它做了 `.[]`/`|=` 迭代。

根因:第 1 项集成里 `applyData`(`app.js`)故意【保留】jq 表达式跨 push 不重置(为了让 `todate`
这类转换在重复 Send 同一接口时延续)。但当新 push 进来的响应【结构不同】时,被保留的表达式会命中
一个现在不存在的路径 → 求值成 `null` → 迭代报错,于是看到的是错误框而非新数据。已用 internal/jq
的 Execute 复现:`.list[]` 对 `{"list":null}`/`{"other":1}`、`.data.items |= ...` 对
`{"data":null}` 均报此错。

修复(`internal/web/app.js`):`runQuery()` 改为返回成功/失败布尔;`applyData` 改成 async——先用
保留的表达式跑一次,若失败且表达式不是 `.`,自动回退成 `.` 再跑一次,保证新数据一定显示。既留住
"同结构重复 Send 延续转换"的好处,又消除"换结构就卡在旧错误"的问题。`applyData` 唯一调用方是
`fetchCurrent`(fire-and-forget),改 async 安全。

验证:go build/install 通过;活链路实测——push 结构 A 后 `.list[]` 正常,push 结构 B 后
`.list[]` 报 `cannot iterate over: null`,客户端回退 `.` 能正确显示 `{"other":42}`。已重新
`go install`。

**6. 重构:开窗判断从"靠服务死活"改为"直接扫 VS Code 有没有 jq-view tab"(2026-08-11 追加)**

问题:第 4/5 项那套"靠 push 是否失败间接判断 tab 在不在 + 服务 idle-exit 联动"方向错了。服务
是后台常驻的,活着≠tab 还在,tab 关了服务也不一定退(实测关窗后 `jq-view -p 8080 -idle-exit 3s`
进程仍在,SSE 连接没断,idle 计时器压根没武装)→ 关窗后再 Send 无法重新唤起。

根因认知:该判断的是"VS Code 里还有没有 jq-view tab",这跟"服务在不在跑"是正交的两件事。

方案(逆向 VS Code 1.116 确认):
- VS Code 1.116 原生 Integrated Browser editor(`Gl`,`workbench.editorinputs.browser`)在扩展
  Tabs API 里 `tab.input` 是 **undefined**——主进程 `_editorInputToDto`(workbench.desktop.main.js)
  没有 browser editor 分支,落 `kind:0`,extHost 侧 `default:return`,所以读不到 viewType/uri。
  (老版 simple-browser 是 webview,input.viewType='mainThreadWebview-simpleBrowser.view'。)
- 两种实现唯一都稳定可读的是 `tab.label`,它来自网页 `<title>`,我们页面正好是 `jq-view`
  (internal/web/index.html)。故统一按 `tab.label.includes('jq-view')` 认,新老版本全覆盖。

改动:
- `.httpyac.js`:新增 `isJqViewTabOpen(vscode)` 扫所有 tabGroups 按 label 认;onResponse 拆成两步
  正交——①`ensureServerUp` 保证服务在跑(先试推,连不上才拉起再推,只管数据);②问 VS Code 有没有
  tab,没有才 `openBrowser` 一次,有就啥都不做靠 SSE 刷新。删掉 idle-exit 相关。
- Go 侧回退:`main.go` 去掉 `-idle-exit` flag;`internal/web/handler.go` 去掉 `Options`/`onIdle`/
  `idleTimer`/`armed`,subscribe/unsubscribe 恢复原样。

验证:go build/vet 通过、`.httpyac.js` 语法 OK;已 `go install` 并杀掉旧的带 idle-exit 进程,当前
无 jq-view 进程,下次 Send 冷启动新二进制。tab.label 判定依据(页面 title=jq-view)已 grep 确认。

**待用户实测确认**:关掉 tab 后再 Send,是否能重新弹出 jq-view;连续 Send 是否只保留一个 tab。

**7. 快捷操作扩展:时间戳 + 嵌套JSON + Base64,一字段一转换(2026-08-11 追加)**

受 ⏱ Time 那个"扫描数据→识别字段→一键生成 jq"模式启发,扩展成三类快捷操作。用户明确两条要求:
①快捷操作放到 jq 输入框【上面单独一行】;②同一字段同时只支持一种转换,【无优先级,后选覆盖之前】。

三类识别器(`internal/web/app.js`,叶子字段逐个扫,各自独立成桶,一个字段可同时进多个桶):
- 时间戳 `detectTimestamp`:数字或纯数字字符串,9-10 位=秒、12-13 位=毫秒。返回 `{unit,isString}`,
  字符串型生成表达式时必须加 `tonumber`(否则 gojq 报 `cannot add: string and number`,已实测)。
  表达式 `path |= (((<num>/[1000])+8*3600) | strftime("%Y-%m-%d %H:%M:%S"))`,北京时间 UTC+8。
- 嵌套 JSON `isJsonString`:trim 后以 `{`/`[` 开头且 `JSON.parse` 成对象/数组。表达式 `path |= (fromjson)`。
- Base64 `isBase64String`:长度≥8 且是 4 的倍数、符合 base64 字符集、`atob` 解出可打印文本;且排除
  本身是 JSON 的(让它归 JSON 类,不重叠)。表达式 `path |= (@base64d)`。

后选覆盖(核心坑):`applyFieldTransform` 对同一 path 已有的 `<path> |= ( ... )` clause 做整段替换,
不同 path 则追加成 `| ` pipeline。★坑:clause 的括号体不能用正则 `\([^)]*\)` 匹配——时间戳表达式含
嵌套括号(`strftime(...)`),正则会在第一个 `)` 断掉,替换后残留 `strftime(...)))` 非法尾巴。改用
`findClauseSpan`:定位 `<path> |=` 后用【括号深度计数】扫到配平的收尾 `)`,拿到完整 [start,end) 再
整段替换。已用 node 复现该 bug 并验证修复。

UI:`internal/web/index.html` footer 改成纵向堆叠——上面 `quick-actions` 行(Quick 标签 + 三个
`quick-select` 下拉,`v-if` 各自按桶是否有字段显示,全空则整行隐藏),下面 `jq-row`(jq 标签+输入+Run)。
`internal/web/style.css` footer 改 `flex-direction:column`,新增 `.quick-actions/.quick-label/
.quick-select/.jq-row`,删掉旧的 `.timestamp-select`。setup 导出改为 `quickFields`(含 timestamp/
json/base64 三桶)+ 三个 apply 方法,删掉旧的 `timestampFields` alias。

验证:纯函数识别器/覆盖逻辑 node 测过;四种时间戳(数字/字符串 × 秒/毫秒)+ JSON + base64 表达式
用真实 gojq 引擎端到端跑通、输出正确(时间戳→北京时间、JSON→对象、base64→hello world);
go build/install 通过;已杀旧进程,下次 Send 冷启动新版。

**扩展方式**:加新快捷操作 = 加一个识别器(判定函数 + collect 里加一桶)+ 一个表达式片段常量 + 一个
apply 方法 + index.html 加一个 `quick-select` 下拉。后选覆盖 `applyFieldTransform` 通用,不用改。
