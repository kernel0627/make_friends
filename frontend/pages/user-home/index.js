const { openPage } = require('../../utils/navigation')
const { getUserHome } = require('../../utils/postApi')
const { decorateHomePost } = require('../../utils/postPresentation')

Page({
  data: {
    userId: '',
    user: null,
    interestTags: [],
    initiatedPosts: [],
    joinedPosts: [],
    initiatedCount: 0,
    joinedCount: 0,
    creditScoreDisplay: 100,
    ratingScoreDisplay: '4.8',
    activeTab: 0,
    loading: false,
  },

  onLoad(options) {
    const userId = (options && options.id) || ''
    if (!userId) {
      wx.showToast({ title: '缺少用户 ID', icon: 'none' })
      return
    }
    this.setData({ userId })
    this.loadUserHome(userId)
  },

  loadUserHome(userId) {
    this.setData({ loading: true })
    getUserHome(userId).then(({ user, initiatedPosts, joinedPosts, interestTags }) => {
      const initiatedList = (initiatedPosts || []).map((item) => decorateHomePost(item, 'author'))
      const joinedList = (joinedPosts || []).map((item) => decorateHomePost(item, 'participant'))
      this.setData({
        user,
        interestTags: Array.isArray(interestTags) ? interestTags : [],
        initiatedPosts: initiatedList,
        joinedPosts: joinedList,
        initiatedCount: initiatedList.length,
        joinedCount: joinedList.length,
        creditScoreDisplay: Number(user.creditScore || 100),
        ratingScoreDisplay: Number(user.ratingScore || 4.8).toFixed(1),
        loading: false,
      })
    }).catch((err) => {
      this.setData({ loading: false })
      wx.showToast({ title: (err && err.message) || '加载用户主页失败', icon: 'none' })
    })
  },

  onTabTap(e) {
    const idx = Number(e.currentTarget.dataset.index || 0)
    this.setData({ activeTab: idx })
  },

  onPostTap(e) {
    const postId = e.currentTarget.dataset.id
    if (!postId) return
    openPage('/pages/detail/index?id=' + postId)
  },
})
