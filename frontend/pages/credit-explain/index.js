const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { getCreditLedger, getUserHome } = require('../../utils/postApi')

function formatTime(ts) {
  const value = Number(ts || 0)
  if (!value) return '--'
  const d = new Date(value)
  const pad = (n) => String(n).padStart(2, '0')
  return pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes())
}

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    userId: '',
    creditScore: 100,
    ratingScore: '5.0',
    ledgerItems: [],
    loading: false,
    rules: [
      '活动完成且双方确认一致：信誉分增加。',
      '活动关闭后 48 小时内完成应评评价：信誉分增加。',
      '主动取消参加会扣少量信誉分。',
      '确认爽约或发起人临时取消已报名活动会扣更多信誉分。',
      '遇到异常会先进入待处理状态，不会立刻扣分。',
    ],
  },

  onLoad(options) {
    const app = getApp()
    const currentUser = app.globalData.currentUser
    const userId = (options && options.id) || (currentUser && currentUser.id) || ''
    this.setData({ userId })
    if (ensurePageLogin(this)) {
      this.loadData()
    }
  },

  onShow() {
    if (ensurePageLogin(this) && this.data.userId) {
      this.loadData()
    }
  },

  loadData() {
    this.setData({ loading: true })
    Promise.all([getCreditLedger(this.data.userId), getUserHome(this.data.userId)])
      .then(([ledgerRes, homeRes]) => {
        this.setData({
          ledgerItems: (ledgerRes.items || []).map((item) =>
            Object.assign({}, item, {
              timeText: formatTime(item.createdAt),
              deltaText: (item.delta > 0 ? '+' : '') + item.delta,
            })
          ),
          creditScore: Number((homeRes.user && homeRes.user.creditScore) || 100),
          ratingScore: Number((homeRes.user && homeRes.user.ratingScore) || 5).toFixed(1),
          loading: false,
        })
      })
      .catch((err) => {
        this.setData({ loading: false })
        wx.showToast({ title: (err && err.message) || '加载信誉说明失败', icon: 'none' })
      })
  },

  onLoginTap() {
    this.setData({ showLoginModal: true })
  },

  onLoginModalClose() {
    this.setData({ showLoginModal: false })
  },

  onWechatLogin() {
    loginWithWechat()
      .then(() => {
        this.setData({ showLoginModal: false })
        if (ensurePageLogin(this)) this.loadData()
      })
      .catch((err) => {
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
})
