const TAB_ROOTS = ['/pages/index/index', '/pages/chat/index', '/pages/profile/index']

function normalizeUrl(url) {
  if (!url) return ''
  return url.startsWith('/') ? url : '/' + url
}

function getUrlPath(url) {
  const normalized = normalizeUrl(url)
  const idx = normalized.indexOf('?')
  return idx >= 0 ? normalized.slice(0, idx) : normalized
}

function getUrlQuery(url) {
  const normalized = normalizeUrl(url)
  const idx = normalized.indexOf('?')
  if (idx < 0) return {}

  return normalized
    .slice(idx + 1)
    .split('&')
    .filter(Boolean)
    .reduce((query, pair) => {
      const equalIdx = pair.indexOf('=')
      const rawKey = equalIdx >= 0 ? pair.slice(0, equalIdx) : pair
      const rawValue = equalIdx >= 0 ? pair.slice(equalIdx + 1) : ''
      const key = decodeURIComponent(rawKey || '')
      if (!key) return query
      query[key] = decodeURIComponent(rawValue || '')
      return query
    }, {})
}

function isTabRoute(url) {
  return TAB_ROOTS.includes(getUrlPath(url))
}

function isCurrentRoute(url) {
  const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
  const current = pages.length ? pages[pages.length - 1] : null
  if (!current || !current.route) return false

  const targetPath = getUrlPath(url)
  const currentPath = '/' + current.route
  if (targetPath !== currentPath) return false

  const targetQuery = getUrlQuery(url)
  const queryKeys = Object.keys(targetQuery)
  if (!queryKeys.length) return true

  const currentOptions = current.options || {}
  return queryKeys.every((key) => String(currentOptions[key] || '') === String(targetQuery[key] || ''))
}

function getCurrentTabRoot() {
  const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
  if (!pages.length) return '/pages/index/index'

  const root = '/' + (pages[0].route || '')
  if (TAB_ROOTS.includes(root)) {
    return root
  }
  return '/pages/index/index'
}

function openPage(url) {
  const normalized = normalizeUrl(url)
  if (!normalized) return
  if (isCurrentRoute(normalized)) return

  if (isTabRoute(normalized)) {
    wx.switchTab({ url: getUrlPath(normalized) })
    return
  }

  wx.navigateTo({ url: normalized })
}

function backToCurrentTabRoot() {
  wx.switchTab({ url: getCurrentTabRoot() })
}

module.exports = {
  TAB_ROOTS,
  getCurrentTabRoot,
  openPage,
  backToCurrentTabRoot,
  isTabRoute,
  isCurrentRoute,
}
