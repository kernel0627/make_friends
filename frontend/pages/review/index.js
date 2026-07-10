const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { submitReviews, getSettlement } = require('../../utils/postApi')

function buildDraft(targets, ratings, comments) {
  const nextRatings = {}
  const nextComments = {}
  ;(targets || []).forEach((target) => {
    const userId = target.user && target.user.id
    if (!userId) return
    nextRatings[userId] = (ratings && ratings[userId]) || 5
    nextComments[userId] = (comments && comments[userId]) || ''
  })
  return { ratings: nextRatings, comments: nextComments }
}

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    postId: '',
    postTitle: '',
    reviewTargets: [],
    ratings: {},
    comments: {},
    loading: false,
    submitting: false,
  },

  onLoad(options) {
    const postId = options && options.id ? options.id : ''
    const title = decodeURIComponent((options && options.title) || '活动评分')
    this.setData({ postId, postTitle: title })
    if (ensurePageLogin(this) && postId) {
      this.loadReviewTargets()
    }
  },

  onShow() {
    if (ensurePageLogin(this) && this.data.postId) {
      this.loadReviewTargets()
    }
  },

  backToProfile() {
    wx.switchTab({ url: '/pages/profile/index' })
  },

  loadReviewTargets() {
    this.setData({ loading: true })
    return getSettlement(this.data.postId)
      .then((settlement) => {
        const reviewTargets = settlement.reviewTargets || []
        const draft = buildDraft(reviewTargets, this.data.ratings, this.data.comments)
        this.setData({
          postTitle: settlement.postTitle || this.data.postTitle,
          reviewTargets,
          ratings: draft.ratings,
          comments: draft.comments,
          loading: false,
        })
        return settlement
      })
      .catch((err) => {
        this.setData({ loading: false })
        wx.showToast({ title: (err && err.message) || '加载评分对象失败', icon: 'none' })
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
        if (ensurePageLogin(this)) this.loadReviewTargets()
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

  onStarTap(e) {
    const userId = e.currentTarget.dataset.userId
    const rating = Number(e.currentTarget.dataset.rating || 0)
    if (!userId || rating < 1 || rating > 5) return
    this.setData({ ['ratings.' + userId]: rating })
  },

  onCommentInput(e) {
    const userId = e.currentTarget.dataset.userId
    if (!userId) return
    this.setData({ ['comments.' + userId]: (e.detail && e.detail.value) || '' })
  },

  onSubmit() {
    if (this.data.submitting) return
    if (!this.data.reviewTargets.length) {
      this.backToProfile()
      return
    }

    const items = this.data.reviewTargets.map((target) => ({
      toUserId: target.user.id,
      rating: Number(this.data.ratings[target.user.id] || 5),
      comment: (this.data.comments[target.user.id] || '').trim(),
    }))

    this.setData({ submitting: true })
    submitReviews(this.data.postId, items)
      .then(() => {
        wx.showToast({ title: '评分提交成功', icon: 'success' })
        setTimeout(() => this.backToProfile(), 240)
      })
      .catch((err) => {
        wx.showToast({ title: (err && err.message) || '评分提交失败', icon: 'none' })
      })
      .finally(() => {
        this.setData({ submitting: false })
      })
  },
})
