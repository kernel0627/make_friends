const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { backToCurrentTabRoot } = require('../../utils/navigation')
const {
  getSettlement,
  submitParticipantSettlement,
  submitAuthorSettlement,
  cancelAllSettlement,
} = require('../../utils/postApi')

function formatTime(ts) {
  const value = Number(ts || 0)
  if (!value) return ''
  const date = new Date(value)
  const pad = (num) => String(num).padStart(2, '0')
  return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
}

function buildStatusText(item) {
  if (!item) return '待履约确认'
  if (item.finalStatus === 'completed') return '已完成'
  if (item.finalStatus === 'cancelled') return '已取消'
  if (item.finalStatus === 'no_show') return '未到场'
  if (item.finalStatus === 'disputed') return '活动异常待处理'
  if (item.participantDecision === 'completed') return '参与者已确认参加，等待发起人处理'
  if (item.participantDecision === 'cancelled') return '参与者已确认取消'
  if (item.participantDecision === 'disputed') return '参与者反馈活动异常'
  return '待履约确认'
}

function participantOptions() {
  return [
    { key: 'completed', label: '我已参加', tone: 'primary' },
    { key: 'cancelled', label: '我已取消', tone: 'warn' },
    { key: 'disputed', label: '活动异常', tone: 'neutral' },
  ]
}

function authorOptions() {
  return [
    { key: 'completed', label: '已到场', tone: 'primary' },
    { key: 'no_show', label: '未到场', tone: 'warn' },
  ]
}

function splitFlowLabel(label) {
  const text = String(label || '').trim().replace(/\s+/g, '')
  if (!text) return []
  if (text.length <= 2) return [text]
  if (text.length === 3) return [text.slice(0, 1), text.slice(1)]
  const middle = Math.ceil(text.length / 2)
  return [text.slice(0, middle), text.slice(middle)].filter(Boolean)
}

function selectionMeta(selectedUserIds, currentItem) {
  const count = Array.isArray(selectedUserIds) ? selectedUserIds.length : 0
  if (count > 1) {
    return {
      summary: '批量处理：已选 ' + count + ' 位成员',
      hint: '提交后会对所有已选成员应用同一履约结果和备注。',
      actionText: '批量处理 ' + count + ' 位成员',
    }
  }
  if (currentItem && currentItem.user) {
    return {
      summary: '当前处理：' + currentItem.user.nickName,
      hint: '当前状态：' + currentItem.statusText,
      actionText: '确认当前成员',
    }
  }
  return {
    summary: '请选择成员',
    hint: '点选一个或多个成员后，可以在底部批量确认履约结果。',
    actionText: '确认当前成员',
  }
}

function submitAuthorSettlementQueue(postId, userIds, payload) {
  return (userIds || []).reduce((chain, userId) => (
    chain.then(() => submitAuthorSettlement(postId, Object.assign({}, payload, { userId })))
  ), Promise.resolve())
}

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    postId: '',
    postTitle: '',
    viewerIsAuthor: false,
    projectCancelled: false,
    canCancelAll: false,
    stage: 'done',
    flowLabel: '',
    flowLabelLines: [],
    pendingMemberCount: 0,
    reviewDeadlineText: '',
    items: [],
    currentItem: null,
    selectedUserId: '',
    selectedUserIds: [],
    selectionCount: 0,
    selectedSummary: '',
    selectedHint: '',
    primaryActionText: '确认当前成员',
    reviewTargets: [],
    noteText: '',
    hasDisputedItems: false,
    loading: false,
    submitting: false,
    showDecisionSheet: false,
    decisionTitle: '',
    decisionOptions: [],
    showCancelAllModal: false,
  },

  onLoad(options) {
    const postId = (options && options.id) || ''
    const title = decodeURIComponent((options && options.title) || '履约处理')
    this.setData({ postId, postTitle: title })
  },

  onShow() {
    if (ensurePageLogin(this) && this.data.postId) {
      this.loadData()
    }
  },

  backToProfile() {
    wx.switchTab({ url: '/pages/profile/index' })
  },

  goReviewPage() {
    wx.redirectTo({
      url: '/pages/review/index?id=' + encodeURIComponent(this.data.postId) + '&title=' + encodeURIComponent(this.data.postTitle || '活动评分'),
      fail: () => {
        backToCurrentTabRoot()
      },
    })
  },

  normalizeItems(items) {
    return (items || []).map((item) => Object.assign({}, item, {
      statusText: buildStatusText(item),
      participantConfirmedText: formatTime(item.participantConfirmedAt),
      authorConfirmedText: formatTime(item.authorConfirmedAt),
      settledText: formatTime(item.settledAt),
    }))
  },

  normalizeSelectedUserIds(items, selectedUserIds, viewerIsAuthor) {
    if (!viewerIsAuthor || !Array.isArray(items) || !items.length) return []
    const itemIds = items.map((item) => item.user && item.user.id).filter(Boolean)
    const selectedSet = new Set(Array.isArray(selectedUserIds) ? selectedUserIds : [])
    const nextSelected = itemIds.filter((id) => selectedSet.has(id))
    return nextSelected
  },

  applySelection(items, selectedUserIds) {
    const selectedSet = new Set(Array.isArray(selectedUserIds) ? selectedUserIds : [])
    return (items || []).map((item) => Object.assign({}, item, {
      selected: !!(item.user && selectedSet.has(item.user.id)),
    }))
  },

  pickCurrentItem(items, viewerIsAuthor, selectedUserIds) {
    if (!Array.isArray(items) || !items.length) return null
    if (!viewerIsAuthor) return items[0]
    const selectedUserId = Array.isArray(selectedUserIds) ? selectedUserIds[0] : ''
    if (!selectedUserId) return null
    return items.find((item) => item.user.id === selectedUserId) || null
  },

  maybeOpenParticipantDecision(stage, viewerIsAuthor, currentItem) {
    if (viewerIsAuthor || stage !== 'settlement' || !currentItem || this.data.submitting) return
    const signature = stage + ':' + (currentItem.user && currentItem.user.id ? currentItem.user.id : '')
    if (this._decisionPromptKey === signature) return
    this._decisionPromptKey = signature
    this.setData({
      showDecisionSheet: true,
      decisionTitle: '确认你的活动情况',
      decisionOptions: participantOptions(),
    })
  },

  loadData() {
    const loadSeq = (this._loadSeq || 0) + 1
    this._loadSeq = loadSeq
    this.setData({ loading: true })
    return getSettlement(this.data.postId)
      .then((res) => {
        if (loadSeq !== this._loadSeq) return res

        const normalizedItems = this.normalizeItems(res.items || [])
        const selectedUserIds = this.normalizeSelectedUserIds(normalizedItems, this.data.selectedUserIds, !!res.viewerIsAuthor)
        const items = this.applySelection(normalizedItems, selectedUserIds)
        const currentItem = this.pickCurrentItem(items, !!res.viewerIsAuthor, selectedUserIds)
        const selectedUserId = currentItem && currentItem.user ? currentItem.user.id : ''
        const meta = selectionMeta(selectedUserIds, currentItem)

        var hasDisputedItems = normalizedItems.some(function(it) { return it.finalStatus === 'disputed' })

        this.setData({
          postTitle: res.postTitle || this.data.postTitle,
          viewerIsAuthor: !!res.viewerIsAuthor,
          projectCancelled: !!res.projectCancelled,
          canCancelAll: !!res.canCancelAll,
          hasDisputedItems: hasDisputedItems,
          stage: res.stage || 'done',
          flowLabel: res.flowLabel || '',
          flowLabelLines: splitFlowLabel(res.flowLabel || ''),
          pendingMemberCount: Number(res.pendingMemberCount || 0),
          reviewDeadlineText: formatTime(res.reviewDeadlineAt),
          items,
          currentItem,
          selectedUserId,
          selectedUserIds,
          selectionCount: selectedUserIds.length,
          selectedSummary: meta.summary,
          selectedHint: meta.hint,
          primaryActionText: meta.actionText,
          reviewTargets: res.reviewTargets || [],
          loading: false,
          noteText: '',
        })

        this.maybeOpenParticipantDecision(res.stage || 'done', !!res.viewerIsAuthor, currentItem)
        return res
      })
      .catch((err) => {
        if (loadSeq !== this._loadSeq) return null

        this.setData({ loading: false })
        wx.showToast({ title: (err && err.message) || '加载履约信息失败', icon: 'none' })
        throw err
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

  onAuthorCardTap(e) {
    if (!this.data.viewerIsAuthor) return
    const userId = e.currentTarget.dataset.userId
    const selectedSet = new Set(this.data.selectedUserIds || [])
    if (selectedSet.has(userId)) {
      selectedSet.delete(userId)
    } else {
      selectedSet.add(userId)
    }
    const selectedUserIds = (this.data.items || [])
      .map((item) => item.user && item.user.id)
      .filter((id) => id && selectedSet.has(id))
    const items = this.applySelection(this.data.items || [], selectedUserIds)
    const currentItem = this.pickCurrentItem(items, true, selectedUserIds)
    const selectedUserId = currentItem && currentItem.user ? currentItem.user.id : ''
    const meta = selectionMeta(selectedUserIds, currentItem)
    this.setData({
      items,
      currentItem,
      selectedUserId,
      selectedUserIds,
      selectionCount: selectedUserIds.length,
      selectedSummary: meta.summary,
      selectedHint: meta.hint,
      primaryActionText: meta.actionText,
    })
  },

  onSelectAllTap() {
    if (!this.data.viewerIsAuthor || this.data.submitting) return
    const selectedUserIds = (this.data.items || []).map((item) => item.user && item.user.id).filter(Boolean)
    const items = this.applySelection(this.data.items || [], selectedUserIds)
    const currentItem = this.pickCurrentItem(items, true, selectedUserIds)
    const selectedUserId = currentItem && currentItem.user ? currentItem.user.id : ''
    const meta = selectionMeta(selectedUserIds, currentItem)
    this.setData({
      items,
      currentItem,
      selectedUserId,
      selectedUserIds,
      selectionCount: selectedUserIds.length,
      selectedSummary: meta.summary,
      selectedHint: meta.hint,
      primaryActionText: meta.actionText,
    })
  },

  onClearSelectionTap() {
    if (!this.data.viewerIsAuthor || this.data.submitting) return
    const selectedUserIds = []
    const items = this.applySelection(this.data.items || [], selectedUserIds)
    const meta = selectionMeta(selectedUserIds, null)
    this.setData({
      items,
      currentItem: null,
      selectedUserId: '',
      selectedUserIds,
      selectionCount: 0,
      selectedSummary: meta.summary,
      selectedHint: meta.hint,
      primaryActionText: meta.actionText,
    })
  },

  onNoteInput(e) {
    this.setData({ noteText: (e.detail && e.detail.value) || '' })
  },

  onPrimaryActionTap() {
    if (this.data.submitting) return
    if (this.data.stage === 'review') {
      this.goReviewPage()
      return
    }
    if (this.data.stage !== 'settlement') {
      this.backToProfile()
      return
    }

    if (this.data.viewerIsAuthor) {
      if (!this.data.selectionCount) {
        wx.showToast({ title: '请先选择要处理的成员', icon: 'none' })
        return
      }
      this.setData({
        showDecisionSheet: true,
        decisionTitle: this.data.selectionCount > 1 ? ('批量确认 ' + this.data.selectionCount + ' 位成员') : '确认该成员的履约情况',
        decisionOptions: authorOptions(),
      })
      return
    }

    this.setData({
      showDecisionSheet: true,
      decisionTitle: '确认你的活动情况',
      decisionOptions: participantOptions(),
    })
  },
  onDecisionSheetClose() {
    if (this.data.submitting) return
    this.setData({ showDecisionSheet: false })
  },

  handleParticipantSettlementResult(decision, res) {
    if (res.projectCancelled || res.stage === 'cancelled') {
      this.backToProfile()
      return
    }

    if (decision === 'completed' && res.reviewTargets && res.reviewTargets.length) {
      this.goReviewPage()
      return
    }
    this.backToProfile()
  },

  onDecisionOptionTap(e) {
    const decision = e.currentTarget.dataset.decision
    if (!decision || this.data.submitting) return
    const note = (this.data.noteText || '').trim()
    const selectedUserIds = (this.data.selectedUserIds || []).filter(Boolean)

    if (this.data.viewerIsAuthor && !selectedUserIds.length) {
      wx.showToast({ title: '请先选择要处理的成员', icon: 'none' })
      return
    }

    this.setData({ submitting: true })
    const request = this.data.viewerIsAuthor
      ? submitAuthorSettlementQueue(this.data.postId, selectedUserIds, {
          decision,
          note,
        })
      : submitParticipantSettlement(this.data.postId, {
          decision,
          note,
        })

    request
      .then(() => {
        if (this.data.viewerIsAuthor && selectedUserIds.length) {
          const processedSet = new Set(selectedUserIds)
          const nextItems = (this.data.items || []).filter((item) => !processedSet.has(item.user.id))
          const nextSelectedUserIds = []
          const decoratedItems = this.applySelection(nextItems, nextSelectedUserIds)
          const nextItem = null
          const meta = selectionMeta(nextSelectedUserIds, nextItem)
          this.setData({
            items: decoratedItems,
            currentItem: nextItem,
            selectedUserId: '',
            selectedUserIds: nextSelectedUserIds,
            selectionCount: nextSelectedUserIds.length,
            selectedSummary: meta.summary,
            selectedHint: meta.hint,
            primaryActionText: meta.actionText,
          })
        }
        this.setData({
          submitting: false,
          showDecisionSheet: false,
          noteText: '',
        })
        wx.showToast({ title: selectedUserIds.length > 1 ? '批量处理成功' : '处理成功', icon: 'success' })
        if (this.data.viewerIsAuthor) {
          return this.loadData()
        }
        return this.loadData().then((res) => this.handleParticipantSettlementResult(decision, res))
      })
      .catch((err) => {
        this.setData({ submitting: false })
        wx.showToast({ title: (err && err.message) || '提交失败', icon: 'none' })
      })
  },
  onCancelAllTap() {
    if (this.data.submitting) return
    this.setData({ showCancelAllModal: true })
  },

  onCancelAllClose() {
    if (this.data.submitting) return
    this.setData({ showCancelAllModal: false })
  },

  onCancelAllConfirm() {
    if (this.data.submitting) return
    this.setData({ submitting: true })
    cancelAllSettlement(this.data.postId)
      .then(() => {
        wx.showToast({ title: '项目已取消', icon: 'success' })
        this.setData({ submitting: false, showCancelAllModal: false })
        this.backToProfile()
      })
      .catch((err) => {
        this.setData({ submitting: false })
        wx.showToast({ title: (err && err.message) || '取消项目失败', icon: 'none' })
      })
  },

  onBackProfileTap() {
    this.backToProfile()
  },
})
