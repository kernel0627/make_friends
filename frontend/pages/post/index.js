const { getCurrentLocation } = require('../../utils/location')
const { validatePostForm, buildTimeInfo } = require('../../utils/postForm')
const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')
const { createPost, getUserHome, generateSmartPostDraft } = require('../../utils/postApi')
const { searchUsers } = require('../../utils/messageApi')
const { searchPlaceSuggestions } = require('../../utils/mapApi')
const { buildSmartPostDraft, buildSmartPostTitle, buildSmartPostDescription } = require('../../utils/smartPostDraft')

const TAG_OPTIONS = {
  运动: ['羽毛球', '足球', '篮球', '跑步', '骑行', '游泳', '其他运动'],
  娱乐: ['桌游', '电影', 'KTV', '游戏', '其他娱乐'],
  学习: ['英语', '考研', '编程', '读书', '其他学习'],
}

function formatRatingScore(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(2) : '暂无'
}

function buildFixedDateTime(dateText, clockText) {
  if (!dateText || !clockText) return null
  const fixedDate = new Date((dateText + ' ' + clockText).replace(/-/g, '/') + ':00')
  if (!Number.isFinite(fixedDate.getTime())) return null
  return fixedDate
}

function normalizeInvitee(user) {
  const source = user || {}
  return {
    id: source.id || '',
    nickName: source.nickName || '未知用户',
    avatarUrl: source.avatarUrl || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
    creditScore: Number(source.creditScore || 100),
    ratingScore: Number(source.ratingScore || 5),
    ratingText: formatRatingScore(source.ratingScore),
  }
}

function mergeInvitees(current, incoming) {
  const map = {}
  ;(current || []).forEach((user) => {
    const normalized = normalizeInvitee(user)
    if (normalized.id) map[normalized.id] = normalized
  })
  ;(incoming || []).forEach((user) => {
    const normalized = normalizeInvitee(user)
    if (normalized.id) map[normalized.id] = normalized
  })
  return Object.keys(map).map((id) => map[id])
}

function askUseSmartLocation() {
  return new Promise((resolve) => {
    wx.showModal({
      title: '智能发布',
      content: '是否使用当前位置判断历史活动地点是否适合参考？跳过定位也可以直接按历史习惯生成。',
      confirmText: '使用定位',
      cancelText: '跳过',
      success(res) {
        resolve(!!(res && res.confirm))
      },
      fail() {
        resolve(false)
      },
    })
  })
}

function askUseNextYearDate(message) {
  return new Promise((resolve) => {
    wx.showModal({
      title: '日期需要确认',
      content: message || '你输入的日期已经过去，是否按明年同一天生成？',
      confirmText: '用明年',
      cancelText: '我修改',
      success(res) {
        resolve(!!(res && res.confirm))
      },
      fail() {
        resolve(false)
      },
    })
  })
}

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    smartInput: '',
    smartGenerating: false,
    smartDraftSummary: [],
    smartTitleHook: null,
    smartDescriptionHook: null,
    title: '',
    description: '',
    category: '',
    subCategory: '',
    timeMode: 'range',
    timeRange: 7,
    fixedTime: '',
    fixedTimeDisplay: '',
    locationMode: 'current',
    locationText: '',
    locationCoords: null,
    locating: false,
    locationSuggestions: [],
    locationSearching: false,
    locationSearchError: '',
    maxCount: 2,
    selectedInvitees: [],
    inviteMessage: '我觉得你可能会对这个活动感兴趣，一起来吗？',
    showInviteModal: false,
    inviteSearchKeyword: '',
    inviteSearchResults: [],
    inviteSearching: false,
    inviteSearchError: '',
    errors: {},
    showCategoryModal: false,
    categoryArray: ['运动', '娱乐', '学习', '其他'],
    showTagModal: false,
    tagOptions: [],
    showTagManualInput: false,
    showDateModal: false,
    selectedDate: '',
    selectedClock: '09:00',
    submitting: false,
  },

  onShow() {
    ensurePageLogin(this)
  },

  onUnload() {
    this._clearLocationSearch()
  },

  onLoginTap() {
    this.setData({ showLoginModal: true })
  },

  onLoginModalClose() {
    this.setData({ showLoginModal: false })
  },

  onWechatLogin() {
    loginWithWechat().then(() => {
      ensurePageLogin(this)
      this.setData({ showLoginModal: false })
      wx.showToast({ title: '登录成功', icon: 'success' })
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

  onTitleInput(e) {
    const title = (e.detail && e.detail.value) || ''
    const hook = this.data.smartTitleHook
    const patch = { title }
    if (hook && hook.active && title !== hook.lastTitle) {
      patch.smartTitleHook = Object.assign({}, hook, { active: false })
    }
    this.setData(patch)
  },
  onDescriptionInput(e) {
    const description = (e.detail && e.detail.value) || ''
    const hook = this.data.smartDescriptionHook
    const patch = { description }
    if (hook && hook.active && description !== hook.lastDescription) {
      patch.smartDescriptionHook = Object.assign({}, hook, { active: false })
    }
    this.setData(patch)
  },

  onSmartInput(e) {
    this.setData({ smartInput: (e.detail && e.detail.value) || '' })
  },

  onSmartGenerate() {
    if (!this.data.isLoggedIn) {
      this.onLoginTap()
      return
    }
    if (this.data.smartGenerating) return
    const input = (this.data.smartInput || '').trim()
    if (!input) {
      wx.showToast({ title: '先输入一句话，例如：明天羽毛球', icon: 'none' })
      return
    }

    this.setData({ smartGenerating: true, smartDraftSummary: [] })
    let currentLocation = null
    let historyFallbackNote = ''
    let smartHistory = { initiatedPosts: [], joinedPosts: [] }
    askUseSmartLocation()
      .then((useLocation) => {
        if (!useLocation) return null
        return getCurrentLocation().catch((err) => {
          const code = err && err.code
          const title = code === 'LOCATION_DENIED'
            ? '未授权定位，将按历史习惯生成'
            : '定位失败，将按历史习惯生成'
          wx.showToast({ title, icon: 'none' })
          return null
        })
      })
      .then((location) => {
        currentLocation = location
        return this._loadSmartHistory()
      })
      .then((history) => {
        smartHistory = history || { initiatedPosts: [], joinedPosts: [] }
        if (history && history._fallbackNote) {
          historyFallbackNote = history._fallbackNote
        }
        return this._generateSmartDraft(input, smartHistory, currentLocation)
      })
      .then((draft) => {
        if (!draft.ok) {
          if (draft.needConfirm && draft.confirmType === 'pastDate') {
            return askUseNextYearDate(draft.message).then((confirmed) => {
              if (!confirmed) {
                wx.showToast({ title: '请修改日期后再生成', icon: 'none' })
                return
              }
              const confirmedDraft = buildSmartPostDraft(input, smartHistory || {}, {
                currentLocation,
                pastDatePolicy: 'nextYear',
              })
              if (!confirmedDraft.ok) {
                wx.showToast({ title: confirmedDraft.error || '暂时无法生成草稿', icon: 'none' })
                return
              }
              if (historyFallbackNote) {
                confirmedDraft.summary = (confirmedDraft.summary || []).concat(historyFallbackNote)
              }
              this._applySmartDraft(confirmedDraft)
              wx.showToast({ title: '已按明年日期生成草稿', icon: 'success' })
            })
          }
          wx.showToast({ title: draft.error || '暂时无法生成草稿', icon: 'none' })
          return
        }
        if (historyFallbackNote) {
          draft.summary = (draft.summary || []).concat(historyFallbackNote)
        }
        this._applySmartDraft(draft)
        wx.showToast({ title: '已生成草稿，请确认后发布', icon: 'success' })
      })
      .catch((err) => {
        wx.showToast({ title: (err && err.message) || '智能生成失败', icon: 'none' })
      })
      .finally(() => {
        this.setData({ smartGenerating: false })
      })
  },

  _generateSmartDraft(input, history, currentLocation) {
    return generateSmartPostDraft(input, { history, currentLocation }).then((draft) => {
      if (draft && draft.ok) return draft
      throw new Error('AI 草稿格式不正确')
    }).catch((err) => {
      const draft = buildSmartPostDraft(input, history || {}, { currentLocation })
      if (draft && draft.ok) {
        const code = err && err.code
        const note = code === 'DEEPSEEK_CONFIG_MISSING'
          ? 'DeepSeek 未配置，已使用本地规则生成。'
          : 'DeepSeek 暂时不可用，已使用本地规则生成。'
        draft.summary = (draft.summary || []).concat(note)
      }
      return draft
    })
  },

  _loadSmartHistory() {
    const app = getApp()
    const user = app && app.globalData && app.globalData.currentUser
    const userId = user && user.id
    if (!userId) {
      return Promise.resolve({ initiatedPosts: [], joinedPosts: [], _fallbackNote: '没有拿到当前用户信息，本次未参考历史活动。' })
    }
    return getUserHome(userId).catch(() => ({
      initiatedPosts: [],
      joinedPosts: [],
      _fallbackNote: '历史活动读取失败，本次主要按输入内容和默认模板生成。',
    }))
  },

  _buildNextActivityInfo(patch) {
    const nextData = Object.assign({}, this.data)
    Object.keys(patch).forEach((key) => {
      if (key.indexOf('.') === -1) nextData[key] = patch[key]
    })
    return nextData
  },

  _withSmartTitleHook(patch) {
    const nextPatch = Object.assign({}, patch)
    const hook = this.data.smartTitleHook
    if (!hook || !hook.active || Object.prototype.hasOwnProperty.call(nextPatch, 'title')) {
      return nextPatch
    }
    if ((this.data.title || '') !== (hook.lastTitle || '')) {
      nextPatch.smartTitleHook = Object.assign({}, hook, { active: false })
      return nextPatch
    }

    const title = buildSmartPostTitle(this._buildNextActivityInfo(nextPatch))
    if (!title) return nextPatch

    nextPatch.title = title
    nextPatch.smartTitleHook = {
      active: true,
      lastTitle: title,
    }
    return nextPatch
  },

  _withSmartDescriptionHook(patch) {
    const nextPatch = Object.assign({}, patch)
    const hook = this.data.smartDescriptionHook
    if (!hook || !hook.active || Object.prototype.hasOwnProperty.call(nextPatch, 'description')) {
      return nextPatch
    }
    if ((this.data.description || '') !== (hook.lastDescription || '')) {
      nextPatch.smartDescriptionHook = Object.assign({}, hook, { active: false })
      return nextPatch
    }

    const description = buildSmartPostDescription(this._buildNextActivityInfo(nextPatch))
    if (!description) return nextPatch

    nextPatch.description = description
    nextPatch.smartDescriptionHook = {
      active: true,
      lastDescription: description,
    }
    return nextPatch
  },

  _setActivityInfoData(patch) {
    this.setData(this._withSmartDescriptionHook(this._withSmartTitleHook(patch)))
  },

  _applySmartDraft(draft) {
    const fields = draft.fields || {}
    const category = fields.category || ''
    const title = fields.title || this.data.title
    const description = fields.description || this.data.description
    this._clearLocationSearch()
    this.setData({
      title,
      description,
      category,
      subCategory: fields.subCategory || '',
      tagOptions: TAG_OPTIONS[category] || [],
      showTagManualInput: category === '其他',
      timeMode: fields.timeMode || 'range',
      timeRange: fields.timeRange || 7,
      fixedTime: fields.fixedTime || '',
      fixedTimeDisplay: fields.fixedTimeDisplay || '',
      selectedDate: fields.selectedDate || '',
      selectedClock: fields.selectedClock || '09:00',
      locationMode: fields.locationMode || 'manual',
      locationText: fields.locationText || '',
      locationCoords: fields.locationCoords || null,
      locationSuggestions: [],
      locationSearchError: '',
      locationSearching: false,
      maxCount: fields.maxCount || 2,
      errors: {},
      smartDraftSummary: draft.summary || [],
      smartTitleHook: fields.title
        ? { active: true, lastTitle: fields.title }
        : null,
      smartDescriptionHook: fields.description
        ? { active: true, lastDescription: fields.description }
        : null,
    })
  },

  onCategoryClick() { this.setData({ showCategoryModal: true }) },
  onCategoryModalClose() { this.setData({ showCategoryModal: false }) },

  onCategorySelect(e) {
    const idx = e.currentTarget.dataset.index
    const category = this.data.categoryArray[idx]
    this._setActivityInfoData({
      category,
      subCategory: '',
      tagOptions: TAG_OPTIONS[category] || [],
      showTagManualInput: category === '其他',
      showCategoryModal: false,
      'errors.category': '',
      'errors.subCategory': '',
    })
  },

  onTagClick() {
    if (!this.data.category || this.data.showTagManualInput) return
    this.setData({ showTagModal: true })
  },
  onTagModalClose() { this.setData({ showTagModal: false }) },
  onTagSelect(e) {
    const idx = e.currentTarget.dataset.index
    this._setActivityInfoData({
      subCategory: this.data.tagOptions[idx] || '',
      showTagModal: false,
      'errors.subCategory': '',
    })
  },
  onTagManualInput(e) {
    this._setActivityInfoData({ subCategory: e.detail.value || '', 'errors.subCategory': '' })
  },

  onTimeModeChange(e) { this._setActivityInfoData({ timeMode: e.detail.value }) },
  onTimeRangeInput(e) { this._setActivityInfoData({ timeRange: parseInt(e.detail.value, 10) || 1 }) },
  onFixedTimeClick() { this.setData({ showDateModal: true }) },
  onInviteCalendarOpen() {
    if (!ensurePageLogin(this)) return
    const snapshot = {
      title: this.data.title || '',
      category: this.data.category || '',
      subCategory: this.data.subCategory || '',
      locationText: this.data.locationText || '',
      locationCoords: this.data.locationCoords || null,
      maxCount: this.data.maxCount || 2,
      timeMode: this.data.timeMode || 'range',
      fixedTime: this.data.fixedTime || '',
      selectedDate: this.data.selectedDate || '',
      selectedClock: this.data.selectedClock || '',
      selectedInvitees: this.data.selectedInvitees || [],
    }
    wx.navigateTo({
      url: '/pages/invite-calendar/index',
      success: (res) => {
        const channel = res.eventChannel
        if (channel && channel.on) {
          channel.on('inviteCalendarApply', (payload) => this.applyInviteCalendarSelection(payload || {}))
        }
        if (channel && channel.emit) {
          channel.emit('initInviteCalendar', { snapshot })
        }
      },
    })
  },
  applyInviteCalendarSelection(payload) {
    const selectedDate = payload.selectedDate || ''
    const selectedClock = payload.selectedClock || '19:00'
    const fixedDate = buildFixedDateTime(selectedDate, selectedClock)
    if (!fixedDate || fixedDate.getTime() <= Date.now()) {
      wx.showToast({ title: '日历返回的时间已过，请重新选择', icon: 'none' })
      return
    }
    const fixedTimeDisplay = selectedDate + ' ' + selectedClock
    this._setActivityInfoData({
      timeMode: 'fixed',
      selectedDate,
      selectedClock,
      fixedTime: fixedDate.toISOString(),
      fixedTimeDisplay,
      selectedInvitees: mergeInvitees(this.data.selectedInvitees, payload.selectedInvitees || []),
      'errors.fixedTime': '',
    })
    wx.showToast({
      title: '已应用日期和邀请',
      icon: 'success',
    })
  },
  onDateModalClose() { this.setData({ showDateModal: false }) },
  onDateChange(e) { this.setData({ selectedDate: e.detail.value }) },
  onClockChange(e) { this.setData({ selectedClock: e.detail.value }) },

  onDateConfirm() {
    if (!this.data.selectedDate || !this.data.selectedClock) {
      wx.showToast({ title: '请选择日期和时间', icon: 'none' })
      return
    }
    const localText = this.data.selectedDate + ' ' + this.data.selectedClock
    const fixedDate = buildFixedDateTime(this.data.selectedDate, this.data.selectedClock)
    if (!fixedDate) {
      wx.showToast({ title: '时间格式不正确', icon: 'none' })
      return
    }
    const fixedTs = fixedDate.getTime()
    const nowTs = Date.now()
    if (fixedTs <= nowTs) {
      wx.showToast({ title: '固定时间必须晚于当前时间', icon: 'none' })
      return
    }

    this._setActivityInfoData({
      fixedTime: fixedDate.toISOString(),
      fixedTimeDisplay: localText,
      showDateModal: false,
      'errors.fixedTime': '',
    })
  },

  onLocationModeChange(e) {
    const mode = e.detail.value
    this._clearLocationSearch()
    this.setData({
      locationMode: mode,
      locationSuggestions: [],
      locationSearchError: '',
      locationSearching: false,
    })
    if (mode === 'current') this._fetchCurrentLocation()
  },

  onGetCurrentLocation() { this._fetchCurrentLocation() },

  _fetchCurrentLocation() {
    if (this.data.locating) return
    this._clearLocationSearch()
    this.setData({ locating: true })
    getCurrentLocation().then((res) => {
      const reverseCode = res.reverseGeocodeError && res.reverseGeocodeError.code
      const hasDetailedAddress = !!(res.resolvedAddress || res.cachedAddress)
      this._setActivityInfoData({
        locationText: res.address || '当前位置',
        locationCoords: { latitude: res.latitude, longitude: res.longitude },
        locationSuggestions: [],
        locationSearchError: '',
        'errors.locationText': '',
      })
      const reverseMessage = res.reverseGeocodeError && res.reverseGeocodeError.errMsg
      wx.showToast({
        title: reverseCode
          ? (hasDetailedAddress ? '已定位，沿用最近地址' : (reverseMessage || '已定位，地址解析失败'))
          : '已获取当前位置',
        icon: reverseCode && !hasDetailedAddress ? 'none' : 'success',
      })
    }).catch((err) => {
      const code = err && err.code
      const title = code === 'LOCATION_DENIED'
        ? '请授权位置信息后重试'
        : code === 'LOCATION_TIMEOUT'
          ? '定位超时，请重试'
          : '获取定位失败，请重试或手动输入'
      wx.showToast({ title, icon: 'none' })
    }).finally(() => {
      this.setData({ locating: false })
    })
  },

  _clearLocationSearch() {
    if (this._locationSearchTimer) {
      clearTimeout(this._locationSearchTimer)
      this._locationSearchTimer = null
    }
    this._locationSearchSeq = (this._locationSearchSeq || 0) + 1
  },

  onLocationInput(e) {
    const value = (e.detail && e.detail.value) || ''
    const keyword = value.trim()
    this._clearLocationSearch()
    this._setActivityInfoData({
      locationText: value,
      locationCoords: null,
      locationSuggestions: [],
      locationSearchError: '',
      'errors.locationText': '',
    })

    if (keyword.length < 2) {
      this.setData({ locationSearching: false })
      return
    }

    const seq = this._locationSearchSeq
    this.setData({ locationSearching: true })
    this._locationSearchTimer = setTimeout(() => {
      this._searchLocationSuggestions(keyword, seq)
    }, 320)
  },

  _searchLocationSuggestions(keyword, seq) {
    searchPlaceSuggestions(keyword, { pageSize: 8 }).then((list) => {
      if (seq !== this._locationSearchSeq) return
      this.setData({
        locationSuggestions: list,
        locationSearching: false,
        locationSearchError: list.length ? '' : '没有找到匹配地点，可以继续手动输入',
      })
    }).catch((err) => {
      if (seq !== this._locationSearchSeq) return
      this.setData({
        locationSuggestions: [],
        locationSearching: false,
        locationSearchError: (err && err.errMsg) || '地点搜索失败，请稍后重试',
      })
    })
  },

  onLocationSuggestionTap(e) {
    const index = Number(e.currentTarget.dataset.index)
    const item = this.data.locationSuggestions[index]
    if (!item) return

    this._clearLocationSearch()
    this._setActivityInfoData({
      locationText: item.title || '',
      locationCoords: item.latitude !== null && item.longitude !== null
        ? { latitude: item.latitude, longitude: item.longitude }
        : null,
      locationSuggestions: [],
      locationSearchError: '',
      locationSearching: false,
      'errors.locationText': '',
    })
  },

  onMaxCountInput(e) { this._setActivityInfoData({ maxCount: parseInt(e.detail.value, 10) || 2 }) },

  noop() {},

  onInviteOpen() {
    this.setData({
      showInviteModal: true,
      inviteSearchKeyword: '',
      inviteSearchResults: [],
      inviteSearchError: '',
    })
    this.searchInviteUsers('')
  },

  onInviteClose() {
    this.setData({ showInviteModal: false })
  },

  onInviteSearchInput(e) {
    this.setData({
      inviteSearchKeyword: (e.detail && e.detail.value) || '',
      inviteSearchError: '',
    })
  },

  onInviteSearchTap() {
    this.searchInviteUsers(this.data.inviteSearchKeyword || '')
  },

  onInviteSearchConfirm() {
    this.searchInviteUsers(this.data.inviteSearchKeyword || '')
  },

  searchInviteUsers(keyword) {
    if (this.data.inviteSearching) return
    this.setData({ inviteSearching: true, inviteSearchError: '' })
    searchUsers((keyword || '').trim(), { limit: 12 }).then((res) => {
      const selectedIds = this.data.selectedInvitees.reduce((acc, user) => {
        acc[user.id] = true
        return acc
      }, {})
      const users = (res.users || []).map((user) => Object.assign({}, user, {
        selected: !!selectedIds[user.id],
        ratingText: formatRatingScore(user.ratingScore),
      }))
      this.setData({
        inviteSearchResults: users,
        inviteSearchError: users.length ? '' : '没有找到可邀请的用户',
      })
    }).catch((err) => {
      this.setData({
        inviteSearchResults: [],
        inviteSearchError: (err && err.message) || '搜索用户失败',
      })
    }).finally(() => {
      this.setData({ inviteSearching: false })
    })
  },

  onInviteUserToggle(e) {
    const userId = e.currentTarget.dataset.id
    if (!userId) return
    const selected = this.data.selectedInvitees.slice()
    const existedIndex = selected.findIndex((user) => user.id === userId)
    if (existedIndex >= 0) {
      selected.splice(existedIndex, 1)
    } else {
      const user = (this.data.inviteSearchResults || []).find((item) => item.id === userId)
      if (user) selected.push(user)
    }
    const selectedIds = selected.reduce((acc, user) => {
      acc[user.id] = true
      return acc
    }, {})
    this.setData({
      selectedInvitees: selected,
      inviteSearchResults: (this.data.inviteSearchResults || []).map((user) => Object.assign({}, user, {
        selected: !!selectedIds[user.id],
      })),
    })
  },

  onInviteRemove(e) {
    const userId = e.currentTarget.dataset.id
    if (!userId) return
    const selectedInvitees = this.data.selectedInvitees.filter((user) => user.id !== userId)
    this.setData({
      selectedInvitees,
      inviteSearchResults: (this.data.inviteSearchResults || []).map((user) => Object.assign({}, user, {
        selected: user.id !== userId && user.selected,
      })),
    })
  },

  onInviteMessageInput(e) {
    this.setData({ inviteMessage: (e.detail && e.detail.value) || '' })
  },

  onInviteConfirm() {
    this.setData({ showInviteModal: false })
  },

  validateForm() {
    return validatePostForm(this.data, { requireSubCategory: true, minMaxCount: 2 })
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

    const timeInfo = buildTimeInfo(this.data.timeMode, this.data.timeRange, this.data.fixedTime)
    const payload = {
      title: (this.data.title || '').trim(),
      description: (this.data.description || '').trim(),
      category: this.data.category,
      subCategory: this.data.subCategory || '',
      timeInfo,
      address: (this.data.locationText || '').trim(),
      coords: this.data.locationCoords
        ? { latitude: this.data.locationCoords.latitude, longitude: this.data.locationCoords.longitude }
        : null,
      maxCount: this.data.maxCount,
      inviteeIds: (this.data.selectedInvitees || []).map((user) => user.id),
      inviteMessage: (this.data.inviteMessage || '').trim(),
    }

    this.setData({ submitting: true })
    createPost(payload).then((post) => {
      const app = getApp()
      const oldPosts = Array.isArray(app.globalData.posts) ? app.globalData.posts : []
      app.globalData.posts = [post].concat(oldPosts.filter((item) => item.id !== post.id))
      wx.showToast({ title: '发布成功', icon: 'success' })
      setTimeout(() => {
        openPage('/pages/detail/index?id=' + post.id)
      }, 250)
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '发布失败', icon: 'none' })
    }).finally(() => {
      this.setData({ submitting: false })
    })
  },
})
