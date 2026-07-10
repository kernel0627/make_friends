const { isTabRoute, getCurrentTabRoot } = require('./utils/navigation')

App({
  onLaunch() {
    this.patchNavigationBehavior()

    const { loadAppData, saveCurrentUser } = require('./utils/store')
    const { getAccessToken, request, clearAuthStorage } = require('./utils/http')

    const localData = loadAppData([], null, {})
    this.globalData.posts = []
    this.globalData.currentUser = getAccessToken() ? (localData.currentUser || null) : null
    this.globalData.joinedPostIds = localData.joinedPostIds
    this.globalData.reviews = localData.reviews
    this.globalData.chatMessages = localData.chatMessages

    if (getAccessToken()) {
      request({ url: '/auth/me', method: 'GET' }).then((res) => {
        const user = res && res.user ? res.user : null
        this.globalData.currentUser = user
        saveCurrentUser(user)
      }).catch(() => {
        clearAuthStorage()
        this.globalData.currentUser = null
        saveCurrentUser(null)
      })
    }
  },

  patchNavigationBehavior() {
    if (wx.__zgbeNavPatched) return

    const rawNavigateTo = wx.navigateTo
    wx.navigateTo = function(options) {
      const opts = options || {}
      const target = (opts.url || '').trim()
      if (!target) {
        return rawNavigateTo.call(wx, opts)
      }

      if (isTabRoute(target)) {
        return wx.switchTab({
          url: target.split('?')[0],
          success: opts.success,
          fail: opts.fail,
          complete: opts.complete,
        })
      }

      return rawNavigateTo.call(wx, opts)
    }

    const rawReLaunch = wx.reLaunch
    wx.reLaunch = function(options) {
      const opts = options || {}
      const target = (opts.url || '').trim()
      if (target && isTabRoute(target)) {
        return wx.switchTab({
          url: target.split('?')[0],
          success: opts.success,
          fail: opts.fail,
          complete: opts.complete,
        })
      }
      return rawReLaunch.call(wx, opts)
    }

    const rawNavigateBack = wx.navigateBack
    wx.navigateBack = function(options) {
      const opts = options || {}
      const pages = typeof getCurrentPages === 'function' ? getCurrentPages() : []
      const delta = Math.max(1, Number(opts.delta || 1))
      if (pages.length <= delta) {
        return wx.switchTab({
          url: getCurrentTabRoot(),
          success: opts.success,
          fail: opts.fail,
          complete: opts.complete,
        })
      }
      return rawNavigateBack.call(wx, opts)
    }

    wx.__zgbeNavPatched = true
  },

  onError(err) {
    console.error('AppError:', err)
  },

  onUnhandledRejection(res) {
    const reason = (res && res.reason) || ''
    console.error('UnhandledRejection:', reason)
  },

  globalData: {
    currentUser: null,
    posts: [],
    joinedPostIds: [],
    reviews: {},
    chatMessages: {},
  },
})
