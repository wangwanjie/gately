// 子路径部署支持：经飞牛 fnOS 统一网关访问时，页面位于 /app/gately/ 之类的前缀下，
// 后端会在 index.html 中注入 <meta name="gately-base">。这里读取该前缀，
// 并给发往本站的绝对路径请求（fetch、XHR、WebSocket、下载链接、新窗口）统一补上前缀。
// 未注入前缀时（Docker、二进制等直接访问）不做任何改动。

/** 路径前缀，例如 /app/gately；直接访问时为空字符串。 */
export const basePath = (document.querySelector('meta[name="gately-base"]')?.getAttribute('content') ?? '').replace(/\/+$/, '')

/** 管理 API 的完整地址前缀（协议 + 主机 + 路径前缀），用于展示与复制。 */
export const apiOrigin = window.location.origin + basePath

/** 给以 / 开头的站内路径补上前缀；已带前缀、协议相对地址（//host）与其他形式保持不变。 */
export function withBase(url: string): string {
  if (!basePath || !url.startsWith('/') || url.startsWith('//')) {
    return url
  }
  if (url === basePath || url.startsWith(basePath + '/')) {
    return url
  }
  return basePath + url
}

// 给指向本站的完整 URL（如 WebSocket 地址）补上前缀
function withBaseAbsolute(url: string): string {
  try {
    const u = new URL(url)
    if (u.host !== window.location.host) {
      return url
    }
    const path = withBase(u.pathname)
    if (path === u.pathname) {
      return url
    }
    u.pathname = path
    return u.toString()
  } catch {
    return url
  }
}

function rewrite(url: string | URL): string | URL {
  if (typeof url !== 'string') {
    return url instanceof URL ? new URL(withBaseAbsolute(url.toString())) : url
  }
  return url.startsWith('/') ? withBase(url) : withBaseAbsolute(url)
}

/**
 * 经网关发往本站的 fetch/XHR 都带上的请求头。经 HTTP 地址打开时浏览器不发 Sec-Fetch-Site，后端凭它确认请求来自
 * 管理界面自己：这是自定义请求头，别的来源的页面要带上它须先通过 CORS 预检，而管理接口不放行预检。
 */
export const pageHeader = 'X-Gately-Page'

/** 判断地址是否指向本站（同协议、主机与端口）。 */
export function isSameOrigin(url: string | URL): boolean {
  try {
    return new URL(url, window.location.href).origin === window.location.origin
  } catch {
    return false
  }
}

/** 给发往本站的请求加上 pageHeader；已带同名请求头时保持不变。 */
export function withPageHeader(input: RequestInfo | URL, init?: RequestInit): [RequestInfo | URL, RequestInit | undefined] {
  if (input instanceof Request) {
    if (!isSameOrigin(input.url) || input.headers.has(pageHeader)) {
      return [input, init]
    }
    const headers = new Headers(init?.headers ?? input.headers)
    headers.set(pageHeader, '1')
    return [input, { ...init, headers }]
  }
  if (!isSameOrigin(input)) {
    return [input, init]
  }
  const headers = new Headers(init?.headers)
  if (!headers.has(pageHeader)) {
    headers.set(pageHeader, '1')
  }
  return [input, { ...init, headers }]
}

function install() {
  const origFetch = window.fetch.bind(window)
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    const target = typeof input === 'string' || input instanceof URL ? rewrite(input) : input
    return origFetch(...withPageHeader(target, init))
  }

  const origOpen = XMLHttpRequest.prototype.open
  XMLHttpRequest.prototype.open = function (this: XMLHttpRequest, method: string, url: string | URL, ...rest: unknown[]) {
    const target = rewrite(url)
    ;(origOpen as (...args: unknown[]) => void).call(this, method, target, ...rest)
    if (isSameOrigin(target)) {
      this.setRequestHeader(pageHeader, '1')
    }
  } as typeof XMLHttpRequest.prototype.open

  const OrigWebSocket = window.WebSocket
  window.WebSocket = class extends OrigWebSocket {
    constructor(url: string | URL, protocols?: string | string[]) {
      super(rewrite(url), protocols)
    }
  }

  const origWindowOpen = window.open.bind(window)
  window.open = (url?: string | URL, target?: string, features?: string) => origWindowOpen(url === undefined ? url : rewrite(url), target, features)

  // 下载链接：模板中的 <a href="/api/..."> 与脚本里创建后直接 click() 的临时链接
  const fixAnchor = (a: HTMLAnchorElement) => {
    const href = a.getAttribute('href')
    if (href && href.startsWith('/') && !href.startsWith('//')) {
      const next = withBase(href)
      if (next !== href) {
        a.setAttribute('href', next)
      }
    }
  }
  document.addEventListener(
    'click',
    (e) => {
      const a = (e.target as Element | null)?.closest?.('a')
      if (a) {
        fixAnchor(a)
      }
    },
    true,
  )
  const origClick = HTMLAnchorElement.prototype.click
  HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
    fixAnchor(this)
    origClick.call(this)
  }
}

if (basePath) {
  install()
}
