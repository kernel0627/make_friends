const { API_BASE_URL } = require('./config')

const ACCESS_TOKEN_KEY = 'zgbe_access_token'
const LEGACY_TOKEN_KEY = 'zgbe_auth_token'
const REFRESH_TOKEN_KEY = 'zgbe_refresh_token'
const ACCESS_EXPIRE_AT_KEY = 'zgbe_access_expire_at'
const REFRESH_EXPIRE_AT_KEY = 'zgbe_refresh_expire_at'
const CURRENT_USER_KEY = 'zgbe_current_user'

const ACCESS_TOKEN_TTL = 7 * 24 * 60 * 60 * 1000
const REFRESH_TOKEN_TTL = 30 * 24 * 60 * 60 * 1000

let refreshingPromise = null
let redirectingToLogin = false

function request(options) {
  return requestWithRetry(Object.assign({}, options || {}))
}

function requestWithRetry(opts) {
  return doRequest(opts).catch((err) => {
    const canRetry =
      err &&
      err.statusCode === 401 &&
      !opts._retryAfterRefresh &&
      !opts.skipAuthRefresh &&
      !isAuthPath(opts.url || '') &&
      !!getRefreshToken()

    if (!canRetry) {
      return Promise.reject(err)
    }

    return refreshAccessToken()
      .then(() => doRequest(Object.assign({}, opts, { _retryAfterRefresh: true })))
      .catch((refreshErr) => {
        handleAuthExpired()
        return Promise.reject(refreshErr)
      })
  })
}

function doRequest(opts) {
  const url = API_BASE_URL + (opts.url || '')
  const method = opts.method || 'GET'
  const header = Object.assign({}, opts.header || {})

  if (!opts.noAuth) {
    const token = getAccessToken()
    if (token && !header.Authorization) {
      header.Authorization = 'Bearer ' + token
    }
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url,
      method,
      data: opts.data || {},
      header,
      timeout: opts.timeout || 12000,
      success(res) {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(res.data)
          return
        }
        reject(buildHttpError(res))
      },
      fail(err) {
        reject(buildNetworkError(err))
      },
    })
  })
}

function buildHttpError(res) {
  const data = (res && res.data) || {}
  const err = new Error(data.error || ('请求失败（' + (res ? res.statusCode : '-') + '）'))
  err.statusCode = res ? res.statusCode : 0
  err.code = data.code || ''
  return err
}

function buildNetworkError(err) {
  const errMsg = (err && err.errMsg) || ''
  const e = new Error('网络请求失败，请稍后再试')
  e.code = 'NETWORK_FAIL'

  if (errMsg.indexOf('timeout') !== -1) {
    e.message = '请求超时，请检查后端服务是否启动'
    e.code = 'NETWORK_TIMEOUT'
    return e
  }

  if (errMsg.indexOf('url not in domain list') !== -1) {
    e.message = '当前请求地址未加入小程序白名单，请检查开发者工具设置'
    e.code = 'REQUEST_DOMAIN_BLOCKED'
    return e
  }

  return e
}

function refreshAccessToken() {
  if (refreshingPromise) return refreshingPromise

  const refreshToken = getRefreshToken()
  if (!refreshToken) {
    return Promise.reject(new Error('缺少刷新令牌，请重新登录'))
  }

  refreshingPromise = doRequest({
    url: '/auth/refresh',
    method: 'POST',
    data: { refreshToken },
    noAuth: true,
    skipAuthRefresh: true,
  })
    .then((res) => {
      const accessToken = (res && (res.accessToken || res.token)) || ''
      const nextRefreshToken = (res && res.refreshToken) || refreshToken
      if (!accessToken) {
        throw new Error('刷新登录状态失败')
      }
      setAuthTokens(accessToken, nextRefreshToken)
      return accessToken
    })
    .finally(() => {
      refreshingPromise = null
    })

  return refreshingPromise
}

function isAuthPath(path) {
  return /^\/auth\//.test(path || '')
}

function getAccessToken() {
  try {
    const token = wx.getStorageSync(ACCESS_TOKEN_KEY) || wx.getStorageSync(LEGACY_TOKEN_KEY) || ''
    if (!token) return ''
    const expireAt = Number(wx.getStorageSync(ACCESS_EXPIRE_AT_KEY) || 0)
    if (expireAt > 0 && Date.now() > expireAt) {
      clearAuthStorage()
      return ''
    }
    return token
  } catch (e) {
    return ''
  }
}

function getRefreshToken() {
  try {
    const token = wx.getStorageSync(REFRESH_TOKEN_KEY) || ''
    if (!token) return ''
    const expireAt = Number(wx.getStorageSync(REFRESH_EXPIRE_AT_KEY) || 0)
    if (expireAt > 0 && Date.now() > expireAt) {
      clearAuthStorage()
      return ''
    }
    return token
  } catch (e) {
    return ''
  }
}

function setAuthTokens(accessToken, refreshToken) {
  const now = Date.now()
  try {
    wx.setStorageSync(ACCESS_TOKEN_KEY, accessToken || '')
    wx.setStorageSync(LEGACY_TOKEN_KEY, accessToken || '')
    wx.setStorageSync(REFRESH_TOKEN_KEY, refreshToken || '')
    wx.setStorageSync(ACCESS_EXPIRE_AT_KEY, accessToken ? now + ACCESS_TOKEN_TTL : 0)
    wx.setStorageSync(REFRESH_EXPIRE_AT_KEY, refreshToken ? now + REFRESH_TOKEN_TTL : 0)
  } catch (e) {}
}

function clearAuthStorage() {
  try {
    wx.setStorageSync(ACCESS_TOKEN_KEY, '')
    wx.setStorageSync(LEGACY_TOKEN_KEY, '')
    wx.setStorageSync(REFRESH_TOKEN_KEY, '')
    wx.setStorageSync(ACCESS_EXPIRE_AT_KEY, 0)
    wx.setStorageSync(REFRESH_EXPIRE_AT_KEY, 0)
    wx.setStorageSync(CURRENT_USER_KEY, null)
  } catch (e) {}

  try {
    const app = getApp()
    if (app && app.globalData) {
      app.globalData.currentUser = null
    }
  } catch (e) {}
}

function handleAuthExpired() {
  clearAuthStorage()
  if (redirectingToLogin) return
  redirectingToLogin = true
  wx.showToast({ title: '登录状态已失效，请重新登录', icon: 'none' })
  wx.switchTab({
    url: '/pages/profile/index',
    complete() {
      redirectingToLogin = false
    },
  })
}

module.exports = {
  request,
  getAccessToken,
  getRefreshToken,
  setAuthTokens,
  clearAuthStorage,
}