const { saveCurrentUser } = require('../../utils/store')
const { loginWithWechat, logout, ensurePageLogin, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')
const { getUserHome, randomizeAvatar, normalizeUser } = require('../../utils/postApi')
const { decorateHomePost } = require('../../utils/postPresentation')

function formatRatingScore(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(2) : '0.00'
}

Page({
  data: {
    currentUser: null,
    isLoggedIn: false,
    activeTab: 0,
    initiatedPosts: [],
    joinedPosts: [],
    creditScore: 0,
    ratingScore: '0.00',
    interestTags: [],
    showLoginModal: false,
    refreshingAvatar: false,
  },

  onShow() {
    const app = getApp()
    const logged = ensurePageLogin(this)
    const currentUser = app.globalData.currentUser
    if (!logged || !currentUser) {
      this.setData({
        currentUser: null,
        isLoggedIn: false,
        initiatedPosts: [],
        joinedPosts: [],
        interestTags: [],
        creditScore: 0,
        ratingScore: '0.00',
      })
      return
    }
    this.loadHomeData(currentUser.id)
  },

  loadHomeData(userId) {
    getUserHome(userId).then(({ user, initiatedPosts, joinedPosts, interestTags }) => {
      const app = getApp()
      const normalizedUser = normalizeUser(user)
      const initiatedList = (initiatedPosts || []).map((item) => decorateHomePost(item, 'author'))
      const joinedList = (joinedPosts || []).map((item) => decorateHomePost(item, 'participant'))

      app.globalData.currentUser = Object.assign({}, app.globalData.currentUser || {}, normalizedUser, {
        id: (user && user.id) || normalizedUser.id,
      })
      saveCurrentUser(app.globalData.currentUser)

      this.setData({
        currentUser: app.globalData.currentUser,
        isLoggedIn: true,
        initiatedPosts: initiatedList,
        joinedPosts: joinedList,
        interestTags: Array.isArray(interestTags) ? interestTags : [],
        creditScore: Number(app.globalData.currentUser.creditScore || 100),
        ratingScore: formatRatingScore(app.globalData.currentUser.ratingScore || 5),
      })
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '加载个人主页失败', icon: 'none' })
    })
  },

  onLogin() {
    this.setData({ showLoginModal: true })
  },

  onLoginModalClose() {
    this.setData({ showLoginModal: false })
  },

  onWechatLogin() {
    loginWithWechat().then((user) => {
      const app = getApp()
      app.globalData.currentUser = user
      saveCurrentUser(user)
      this.setData({
        currentUser: user,
        isLoggedIn: true,
        showLoginModal: false,
      })
      wx.showToast({ title: '登录成功', icon: 'success' })
      this.onShow()
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '登录失败', icon: 'none' })
    })
  },

  onPasswordLogin() {
    this.setData({ showLoginModal: false })
    gotoLoginPage()
  },

  onRegister() {
    this.setData({ showLoginModal: false })
    gotoRegisterPage()
  },

  onRandomAvatarTap() {
    if (!this.data.isLoggedIn || this.data.refreshingAvatar) return

    this.setData({ refreshingAvatar: true })
    randomizeAvatar().then((user) => {
      const app = getApp()
      const current = Object.assign({}, app.globalData.currentUser || {}, user)
      app.globalData.currentUser = current
      saveCurrentUser(current)

      const posts = Array.isArray(app.globalData.posts) ? app.globalData.posts.slice() : []
      for (let i = 0; i < posts.length; i += 1) {
        const post = posts[i]
        if (post.author && post.author.id === current.id) {
          post.author.avatarUrl = current.avatarUrl
        }
        if (Array.isArray(post.joinedUsers)) {
          post.joinedUsers = post.joinedUsers.map((item) => {
            if (item.id !== current.id) return item
            return Object.assign({}, item, { avatarUrl: current.avatarUrl })
          })
        }
      }
      app.globalData.posts = posts

      this.setData({
        currentUser: current,
        creditScore: Number(current.creditScore || 100),
        ratingScore: formatRatingScore(current.ratingScore || 5),
      })
      wx.showToast({ title: '头像已刷新', icon: 'success' })
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '刷新头像失败', icon: 'none' })
    }).finally(() => {
      this.setData({ refreshingAvatar: false })
    })
  },

  onLogoutTap() {
    logout().then(() => {
      this.setData({
        currentUser: null,
        isLoggedIn: false,
        initiatedPosts: [],
        joinedPosts: [],
        creditScore: 0,
        ratingScore: '0.00',
      })
      wx.showToast({ title: '已退出登录', icon: 'success' })
    })
  },

  onPostTap(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return
    openPage('/pages/detail/index?id=' + id)
  },

  onActionTap(e) {
    const route = e.currentTarget.dataset.route
    if (!route) return
    openPage(route)
  },

  onCreditExplainTap() {
    const userId = this.data.currentUser && this.data.currentUser.id
    if (!userId) return
    openPage('/pages/credit-explain/index?id=' + encodeURIComponent(userId))
  },

  onTabTap(e) {
    const idx = Number(e.currentTarget.dataset.index || 0)
    this.setData({ activeTab: idx })
  },

  onReviewTap(e) {
    const id = e.currentTarget.dataset.id
    const title = e.currentTarget.dataset.title
    if (!id) return
    openPage('/pages/review/index?id=' + id + '&title=' + encodeURIComponent(title || '活动评分'))
  },
})
