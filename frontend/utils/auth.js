const { request, getAccessToken, getRefreshToken, setAuthTokens, clearAuthStorage } = require('./http')
const { saveCurrentUser } = require('./store')
const { USE_MOCK_AUTH } = require('./config')
const { openPage } = require('./navigation')

const LOCAL_ACCOUNT_KEY = 'zgbe_local_accounts'
const LOGIN_REDIRECT_KEY = 'zgbe_login_redirect'
const DEFAULT_MOCK_ADMIN = { nickname: 'admin', password: '123456' }

function isLoggedIn() {
  const app = getApp()
  return !!(app && app.globalData && app.globalData.currentUser && getAccessToken())
}

function ensurePageLogin(page) {
  const logged = isLoggedIn()
  if (!logged) {
    const app = getApp()
    if (app && app.globalData) {
      app.globalData.currentUser = null
    }
    saveCurrentUser(null)
  }
  if (page && typeof page.setData === 'function') {
    page.setData({ isLoggedIn: logged })
  }
  return logged
}

function loginWithWechat() {
  if (USE_MOCK_AUTH) {
    ensureMockAdminAccount()
    return Promise.resolve(mockLoginByNickname('微信游客'))
  }

  return wxLogin()
    .then((code) => request({
      url: '/auth/wechat-login',
      method: 'POST',
      data: { code },
      skipAuthRefresh: true,
    }))
    .then(handleLoginResponse)
}

function passwordLogin(nickname, password) {
  if (USE_MOCK_AUTH) {
    ensureMockAdminAccount()
    return Promise.resolve(mockPasswordLogin(nickname, password))
  }

  return request({
    url: '/auth/password-login',
    method: 'POST',
    data: { nickname: nickname || '', password: password || '' },
    skipAuthRefresh: true,
  }).then(handleLoginResponse)
}

function registerWithPassword(nickname, password) {
  if (USE_MOCK_AUTH) {
    ensureMockAdminAccount()
    return Promise.resolve(mockRegister(nickname, password))
  }

  return request({
    url: '/auth/register',
    method: 'POST',
    data: { nickname: nickname || '', password: password || '' },
    skipAuthRefresh: true,
  }).then(handleLoginResponse)
}

function handleLoginResponse(res) {
  const accessToken = (res && (res.accessToken || res.token)) || ''
  const refreshToken = (res && res.refreshToken) || ''
  if (!res || !res.user || !accessToken) {
    throw new Error('登录失败，请稍后重试')
  }
  setAuthTokens(accessToken, refreshToken)
  const app = getApp()
  if (app && app.globalData) {
    app.globalData.currentUser = res.user
  }
  saveCurrentUser(res.user)
  return res.user
}

function logout() {
  if (USE_MOCK_AUTH) {
    clearAuthStorage()
    saveCurrentUser(null)
    return Promise.resolve()
  }

  const refreshToken = getRefreshToken()
  return request({
    url: '/auth/logout',
    method: 'POST',
    data: { refreshToken },
    skipAuthRefresh: true,
  })
    .catch(() => null)
    .then(() => {
      clearAuthStorage()
      saveCurrentUser(null)
    })
}

function gotoLoginPage(redirectUrl) {
  if (redirectUrl) saveLoginRedirect(redirectUrl)
  openPage('/pages/login/index')
}

function gotoRegisterPage(redirectUrl) {
  if (redirectUrl) saveLoginRedirect(redirectUrl)
  openPage('/pages/register/index')
}

function saveLoginRedirect(url) {
  const target = (url || '').trim()
  if (!target) return
  try {
    wx.setStorageSync(LOGIN_REDIRECT_KEY, target)
  } catch (e) {}
}

function consumeLoginRedirect() {
  try {
    const target = (wx.getStorageSync(LOGIN_REDIRECT_KEY) || '').trim()
    wx.removeStorageSync(LOGIN_REDIRECT_KEY)
    return target
  } catch (e) {
    return ''
  }
}

function mockRegister(nickname, password) {
  const name = (nickname || '').trim()
  const pwd = (password || '').trim()
  if (!name || !pwd) {
    throw new Error('请输入昵称和密码')
  }
  if (pwd.length < 6) {
    throw new Error('密码至少需要 6 位')
  }
  const accounts = getLocalAccounts()
  if (accounts.some((item) => item.nickname === name)) {
    throw new Error('昵称已经存在')
  }
  accounts.push({ nickname: name, password: pwd })
  setLocalAccounts(accounts)
  return mockLoginByNickname(name)
}

function mockPasswordLogin(nickname, password) {
  const name = (nickname || '').trim()
  const pwd = (password || '').trim()
  if (!name || !pwd) {
    throw new Error('请输入昵称和密码')
  }
  const accounts = getLocalAccounts()
  const matched = accounts.find((item) => item.nickname === name && item.password === pwd)
  if (!matched) {
    throw new Error('昵称或密码错误')
  }
  return mockLoginByNickname(name)
}

function mockLoginByNickname(nickname) {
  const name = (nickname || '').trim() || '微信游客'
  const now = Date.now()
  const user = {
    id: 'user_local_' + name,
    platform: 'mock',
    nickName: name,
    avatarUrl: 'https://api.dicebear.com/7.x/avataaars/svg?seed=' + encodeURIComponent(name),
    creditScore: 100,
    ratingScore: 5,
    createdAt: now,
    updatedAt: now,
  }
  return handleLoginResponse({
    token: 'mock_access_' + now,
    accessToken: 'mock_access_' + now,
    refreshToken: 'mock_refresh_' + now,
    user,
  })
}

function getLocalAccounts() {
  try {
    const value = wx.getStorageSync(LOCAL_ACCOUNT_KEY)
    return Array.isArray(value) ? value : []
  } catch (e) {
    return []
  }
}

function setLocalAccounts(list) {
  try {
    wx.setStorageSync(LOCAL_ACCOUNT_KEY, list)
  } catch (e) {}
}

function ensureMockAdminAccount() {
  const accounts = getLocalAccounts()
  if (accounts.some((item) => item.nickname === DEFAULT_MOCK_ADMIN.nickname)) return
  accounts.push(DEFAULT_MOCK_ADMIN)
  setLocalAccounts(accounts)
}

function wxLogin() {
  return new Promise((resolve, reject) => {
    wx.login({
      success(res) {
        if (res && res.code) {
          resolve(res.code)
          return
        }
        reject(new Error('未获取到微信登录凭证'))
      },
      fail(err) {
        reject(err || new Error('微信登录失败'))
      },
    })
  })
}

module.exports = {
  ensurePageLogin,
  loginWithWechat,
  passwordLogin,
  registerWithPassword,
  logout,
  isLoggedIn,
  gotoLoginPage,
  gotoRegisterPage,
  consumeLoginRedirect,
}