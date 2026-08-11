#!/usr/bin/env bash
# 启动一个常驻的 jq-view,供 httpYac 的 .httpyac.js hook 推送响应。
# 端口默认 8080,和 .httpyac.js 里的 JQVIEW_URL 对应。
#
# 默认不弹外部浏览器 —— 推荐在 VS Code 里用内置 Simple Browser 打开
# http://localhost:8080,点 send 后就地实时刷新,不用切窗口。
# 传第二个参数 open 才弹外部浏览器: ./start-jqview.sh 8080 open
set -euo pipefail

PORT="${1:-8080}"
OPEN="${2:-}"

if [ "$OPEN" = "open" ]; then
  echo '{}' | jq-view -p "$PORT" -open
else
  echo '{}' | jq-view -p "$PORT"
fi
