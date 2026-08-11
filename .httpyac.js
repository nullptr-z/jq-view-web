// httpYac ↔ jq-view 集成 (方案 B + C，全自动)
//
// 用法:把本文件拷到你实际用 httpYac 的【项目根目录】。
// 之后只管点 Send —— 服务没起会自动拉起、浏览器自动打开、后续请求复用同一个
// tab 实时刷新。时间戳在 jq-view 的 jq 输入框用 `todate` / `strftime` 就地转。
//
// 唯一前置:jq-view 已装进 PATH(本仓库根目录 `go install .`)。
//
// hook 不阻断 httpYac 自身的响应展示 —— VS Code 里照常显示,jq-view 只是额外的可视化。

const { spawn } = require('child_process');

const JQVIEW_URL = process.env.JQVIEW_URL || 'http://localhost:8080';
const PORT = (() => {
  const m = JQVIEW_URL.match(/:(\d+)/);
  return m ? m[1] : '8080';
})();

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// 只 POST 一次,连不上就抛错。
async function push(body) {
  await fetch(`${JQVIEW_URL}/api/push`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
  });
}

// vscode-httpyac 在 VS Code 扩展宿主进程内跑 httpYac,同进程里 require('vscode') 能拿到真实
// VS Code API(纯 CLI/非 VS Code 场景下会抛错,catch 掉即可)。
function getVSCodeApi() {
  try {
    // eslint-disable-next-line global-require
    return require('vscode');
  } catch (_) {
    return null;
  }
}

// 页面 <title> 是 "jq-view"(见 internal/web/index.html),VS Code 会把它当作 tab 的 label。
const TAB_LABEL = 'jq-view';

// 直接问 VS Code:现在还有没有一个 jq-view 的浏览器 tab 开着?
//
// 为什么靠 label 而不是 viewType/uri:VS Code 1.116 的原生 Integrated Browser editor 在扩展
// Tabs API 里 `tab.input` 是 undefined(主进程 _editorInputToDto 没有 browser editor 分支,落
// kind:0,没有 viewType/uri 可读);而老版 simple-browser 是 webview,input.viewType 是
// 'mainThreadWebview-simpleBrowser.view'。两种实现唯一都稳定可读的就是 tab.label——它来自网页
// <title>,我们的页面正好是 "jq-view"。所以统一按 label 含 "jq-view" 来认,新老版本都覆盖。
//
// 这一步跟"服务在不在跑"完全正交:服务是后台常驻的,活着不代表 tab 还在,tab 关了服务也不一定
// 退。要不要开窗只由"tab 在不在"决定,不再靠服务死活或 SSE 连接来间接推断。
function isJqViewTabOpen(vscode) {
  try {
    for (const group of vscode.window.tabGroups.all) {
      for (const tab of group.tabs) {
        if (tab.label && tab.label.includes(TAB_LABEL)) {
          return true;
        }
      }
    }
    return false;
  } catch (_) {
    // 拿不到 tabGroups(极旧版本 VS Code):保守当作"开着",不去抢弹窗。
    return true;
  }
}

// 弹出 VS Code 内置浏览器展示 jq-view。老版 VS Code 走 simple-browser 扩展的
// `simpleBrowser.show`(webview 单实例);VS Code 1.116+ 该命令转发给原生 Integrated Browser。
async function openBrowser(vscode, url) {
  try {
    await vscode.commands.executeCommand('simpleBrowser.show', url, { preserveFocus: true });
  } catch (_) {
    /* 命令不可用:静默放过 */
  }
}

// 后台拉起一个常驻 jq-view(不随 httpYac 退出)。
// 拿得到 vscode API 时不传 -open(浏览器由 VS Code 内置面板弹,见 openBrowser);拿不到(纯 CLI
// 场景)才 -open 让 Go 侧自己 exec `open` 弹系统默认浏览器。
function startServer(openExternally) {
  const args = ['-p', PORT];
  if (openExternally) {
    args.push('-open');
  }
  const p = spawn('jq-view', args, {
    stdio: ['pipe', 'ignore', 'ignore'],
    detached: true,
  });
  p.stdin.write('{}'); // 占位数据,等真实响应 push 过来再刷新
  p.stdin.end();
  p.unref();
}

// 确保服务在跑并且能连上,返回后即可 push。只负责"服务",不管"窗口"。
// starting 只在同一次 Send 触发的并发请求间去重(.httpyac.js 每次 Send 被重新 require,不跨
// Send 保留状态,也不需要)。
let starting = null;
function ensureServerUp(openExternally) {
  if (!starting) {
    startServer(openExternally);
    starting = (async () => {
      for (let i = 0; i < 20; i++) {
        await sleep(300);
        try {
          await fetch(`${JQVIEW_URL}/api/current`);
          return;
        } catch (_) {
          /* 还没起来,继续等 */
        }
      }
    })();
  }
  return starting;
}

module.exports = {
  configureHooks(api) {
    api.hooks.onResponse.addHook('pushToJqView', async (response) => {
      // 只处理 JSON 响应,其它(HTML/文本/二进制)放过。
      const mime = response?.contentType?.mimeType || '';
      if (!mime.includes('json') || response.parsedBody === undefined) {
        return; // 返回 undefined = 不改变 httpYac 原有行为
      }

      const body = JSON.stringify(response.parsedBody);
      const vscode = getVSCodeApi();

      try {
        // 1) 确保服务在跑:先试推,连不上就拉起再推。这一步只保证数据到位。
        try {
          await push(body);
        } catch (_) {
          await ensureServerUp(!vscode);
          await push(body);
        }

        // 2) 确保有窗口:直接问 VS Code 还有没有 jq-view tab。没有(从没开过 / 被手动关了)才开
        //    一次;有就什么都不做,tab 靠 SSE 自动刷新。开窗与否只看 tab 在不在,与服务死活无关。
        if (vscode && !isJqViewTabOpen(vscode)) {
          await openBrowser(vscode, `${JQVIEW_URL}?sidebar=collapsed`);
        }
      } catch (_) {
        // jq-view 不在 PATH 等极端情况:静默放过,不影响 httpYac。
      }
      // 不 return response,保持 httpYac 自己的输出。
    });
  },
};
