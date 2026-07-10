const { getCurrentLocation } = require('../../utils/location')
const { validatePostForm, buildTimeInfo } = require('../../utils/postForm')
const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')
const { getPostDetail, updatePost } = require('../../utils/postApi')

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    postId: '',
    title: '',
    description: '',
    category: '',
    subCategory: '',
    timeMode: 'range',
    timeRange: 7,
    fixedTime: '',
    fixedTimeDisplay: '',
    locationMode: 'manual',
    locationText: '',
    locationCoords: null,
    maxCount: 2,
    postCurrentCount: 0,
    errors: {},
    showDateModal: false,
    selectedDate: '',
    selectedClock: '09:00',
    submitting: false,
  },

  onLoad(options) {
    if (!ensurePageLogin(this)) return
    const postId = (options && options.id) || ''
    if (!postId) {
      wx.showToast({ title: '活动不存在', icon: 'none' })
      return
    }
    this.setData({ postId })
    this.loadPost(postId)
  },

  onShow() {
    ensurePageLogin(this)
  },

  loadPost(postId) {
    getPostDetail(postId).then(({ post }) => {
      const app = getApp()
      const currentUser = app.globalData.currentUser
      const isAuthor = !!(
        post.viewerIsAuthor ||
        (currentUser && (
          (post.author && post.author.id === currentUser.id) ||
          post.authorId === currentUser.id
        ))
      )
      if (!currentUser || !isAuthor) {
        wx.showToast({ title: '无权限编辑', icon: 'none' })
        return
      }

      const fixedTime = post.timeInfo && post.timeInfo.mode === 'fixed' ? post.timeInfo.fixedTime : ''
      const fixedDate = fixedTime ? new Date(fixedTime) : null
      const selectedDate = fixedDate
        ? `${fixedDate.getFullYear()}-${String(fixedDate.getMonth() + 1).padStart(2, '0')}-${String(fixedDate.getDate()).padStart(2, '0')}`
        : ''
      const selectedClock = fixedDate
        ? `${String(fixedDate.getHours()).padStart(2, '0')}:${String(fixedDate.getMinutes()).padStart(2, '0')}`
        : '09:00'

      this.setData({
        title: post.title || '',
        description: post.description || '',
        category: post.category || '',
        subCategory: post.subCategory || '',
        timeMode: post.timeInfo ? post.timeInfo.mode : 'range',
        timeRange: post.timeInfo && post.timeInfo.days ? post.timeInfo.days : 7,
        fixedTime,
        fixedTimeDisplay: fixedTime ? `${selectedDate} ${selectedClock}` : '',
        selectedDate,
        selectedClock,
        locationText: post.address || '',
        locationCoords: post.coords || null,
        locationMode: post.coords ? 'current' : 'manual',
        maxCount: post.maxCount || 2,
        postCurrentCount: post.currentCount || 0,
      })
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '加载活动失败', icon: 'none' })
    })
  },

  onLoginTap() {
    this.setData({ showLoginModal: true })
  },

  onLoginModalClose() {
    this.setData({ showLoginModal: false })
  },

  onWechatLogin() {
    loginWithWechat().then(() => {
      this.setData({ showLoginModal: false })
      ensurePageLogin(this)
      if (this.data.postId) this.loadPost(this.data.postId)
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

  onTitleInput(e) { this.setData({ title: e.detail.value }) },
  onDescriptionInput(e) { this.setData({ description: e.detail.value }) },
  onSubCategoryInput(e) { this.setData({ subCategory: e.detail.value }) },
  onTimeModeChange(e) { this.setData({ timeMode: e.detail.value }) },
  onTimeRangeInput(e) { this.setData({ timeRange: parseInt(e.detail.value, 10) || 1 }) },
  onFixedTimeClick() { this.setData({ showDateModal: true }) },
  onDateModalClose() { this.setData({ showDateModal: false }) },
  onDateChange(e) { this.setData({ selectedDate: e.detail.value }) },
  onClockChange(e) { this.setData({ selectedClock: e.detail.value }) },

  onDateConfirm() {
    if (!this.data.selectedDate || !this.data.selectedClock) {
      wx.showToast({ title: '请选择日期和时间', icon: 'none' })
      return
    }
    const localText = this.data.selectedDate + ' ' + this.data.selectedClock
    const fixedDate = new Date(localText.replace(/-/g, '/') + ':00')
    const fixedTs = fixedDate.getTime()
    const nowTs = Date.now()
    if (!Number.isFinite(fixedTs)) {
      wx.showToast({ title: '时间格式不正确', icon: 'none' })
      return
    }
    if (fixedTs <= nowTs) {
      wx.showToast({ title: '固定时间必须晚于当前时间', icon: 'none' })
      return
    }
    this.setData({
      fixedTime: fixedDate.toISOString(),
      fixedTimeDisplay: localText,
      showDateModal: false,
      'errors.fixedTime': '',
    })
  },

  onLocationModeChange(e) {
    const mode = e.detail.value
    this.setData({ locationMode: mode })
    if (mode === 'current') this._fetchCurrentLocation()
  },

  onGetCurrentLocation() { this._fetchCurrentLocation() },

  _fetchCurrentLocation() {
    getCurrentLocation().then((res) => {
      this.setData({
        locationText: res.address || '当前位置',
        locationCoords: { latitude: res.latitude, longitude: res.longitude },
      })
    }).catch(() => {
      this.setData({ locationMode: 'manual' })
    })
  },

  onLocationInput(e) { this.setData({ locationText: e.detail.value }) },
  onMaxCountInput(e) { this.setData({ maxCount: parseInt(e.detail.value, 10) || 2 }) },

  validateForm() {
    return validatePostForm(this.data, {
      requireSubCategory: false,
      minMaxCount: Math.max(2, this.data.postCurrentCount || 0),
    })
  },

  onSubmit() {
    if (!this.data.isLoggedIn) {
      this.onLoginTap()
      return
    }
    if (this.data.submitting) return

    const result = this.validateForm()
    if (!result.valid) {
      this.setData({ errors: result.errors })
      wx.showToast({ title: '请检查输入', icon: 'none' })
      return
    }

    const payload = {
      title: (this.data.title || '').trim(),
      description: (this.data.description || '').trim(),
      category: this.data.category,
      subCategory: this.data.subCategory || '',
      timeInfo: buildTimeInfo(this.data.timeMode, this.data.timeRange, this.data.fixedTime),
      address: (this.data.locationText || '').trim(),
      coords: this.data.locationCoords
        ? { latitude: this.data.locationCoords.latitude, longitude: this.data.locationCoords.longitude }
        : null,
      maxCount: this.data.maxCount,
    }

    this.setData({ submitting: true })
    updatePost(this.data.postId, payload).then((post) => {
      const app = getApp()
      const list = Array.isArray(app.globalData.posts) ? app.globalData.posts.slice() : []
      const idx = list.findIndex((item) => item.id === post.id)
      if (idx >= 0) {
        list[idx] = post
      } else {
        list.unshift(post)
      }
      app.globalData.posts = list
      wx.showToast({ title: '保存成功', icon: 'success' })
      setTimeout(() => {
        openPage('/pages/detail/index?id=' + this.data.postId)
      }, 250)
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '保存失败', icon: 'none' })
    }).finally(() => {
      this.setData({ submitting: false })
    })
  },
})
