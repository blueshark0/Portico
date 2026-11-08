package desktop

import (
	"net/http"
	"net/url"
)

// loadingPage 是桌面窗口在连接到 portico-server 之前显示的本地页面（由 Wails 资源服务器提供）。
// 连接失败时 Shell 以 ?error= 重新加载该页面展示原因；错误文本通过 textContent 写入，不解析 HTML。
const loadingPage = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>Portico</title>
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans SC", sans-serif;
    background: #1b2636; color: #e7ecf3;
  }
  main { max-width: 520px; padding: 32px; text-align: center; }
  .spinner {
    width: 28px; height: 28px; margin: 0 auto 18px; border-radius: 50%;
    border: 3px solid rgba(255,255,255,.18); border-top-color: #356df3;
    animation: spin .9s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  h1 { font-size: 17px; font-weight: 600; margin: 0 0 8px; }
  p { font-size: 13px; line-height: 1.7; margin: 0; color: #a9b4c4; }
  pre {
    margin: 16px 0 0; padding: 12px 14px; border-radius: 8px; text-align: left;
    white-space: pre-wrap; word-break: break-word; font-size: 12px;
    background: rgba(214,75,75,.12); border: 1px solid rgba(214,75,75,.4); color: #f2b8b8;
  }
  .error .spinner { display: none; }
  .hidden { display: none; }
</style>
</head>
<body>
<main id="root">
  <div class="spinner"></div>
  <h1 id="title">正在启动 Portico 服务…</h1>
  <p id="hint">首次启动可能需要几秒钟。</p>
  <pre id="detail" class="hidden"></pre>
</main>
<script>
  const error = new URLSearchParams(location.search).get('error');
  if (error) {
    document.getElementById('root').classList.add('error');
    document.getElementById('title').textContent = '无法连接 Portico 服务';
    document.getElementById('hint').textContent = '可在托盘菜单中选择「重新连接服务」重试；仍失败请查看日志。';
    const detail = document.getElementById('detail');
    detail.textContent = error;
    detail.classList.remove('hidden');
  }
</script>
</body>
</html>
`

// LoadingAssets 返回桌面应用的资源处理器：只提供启动 / 错误页面，业务 UI 由 portico-server 提供。
func LoadingAssets() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(loadingPage))
	})
}

func errorPageURL(message string) string {
	return "/?error=" + url.QueryEscape(message)
}
