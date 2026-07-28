const { openPage } = require('../../utils/navigation')
const {
  getPostDetail,
  getSettlement,
  getPostModeration,
  reportPost,
  appealModeration,
  joinPost,
  closePost,
} = require('../../utils/postApi')
const { loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { closedAtText, formatPostTime, formatRatingDisplay } = require('../../utils/postPresentation')

function resolveAuthorId(post) {
  return (post && ((post.author && post.author.id) || post.authorId)) || ''
}

function formatRatingScore(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(2) : '0.00'
}

function decorateDetailPost(post) {
  if (!post) return post
  const author = Object.assign({}, post.author || {}, {
    ratingScoreDisplay: formatRatingScore(post.author && post.author.ratingScore),
  })
  return Object.assign({}, post, { author })
}

function buildClosedSummary(post) {
  if (!post || post.status !== 'closed') return ''
  return closedAtText(post)
}

function hasReviewTargets(settlementInfo) {
  return !!(settlementInfo && Array.isArray(settlementInfo.reviewTargets) && settlementInfo.reviewTargets.length)
}

function hasAuthorSettlementWork(settlementInfo) {
  if (!settlementInfo || settlementInfo.projectCancelled) return false
  if (Number(settlementInfo.pendingMemberCount || 0) > 0) return true
  if (!Array.isArray(settlementInfo.items)) return false
  return settlementInfo.items.some((item) => {
    if (!item) return false
    if (item.state && item.state.canAuthorConfirm) return true
    const status = String(item.finalStatus || '').trim()
    return status === '' || status === 'pending' || status === 'disputed'
  })
}

function hasParticipantSettlementWork(settlementInfo) {
  if (!settlementInfo || settlementInfo.projectCancelled || !Array.isArray(settlementInfo.items)) return false
  return settlementInfo.items.some((item) => {
    if (!item) return false
    if (item.state && item.state.canParticipantConfirm) return true
    const status = String(item.finalStatus || '').trim()
    return status === '' || status === 'pending' || status === 'disputed'
  })
}

function buildParticipantResultText(settlementInfo) {
  const item = settlementInfo && Array.isArray(settlementInfo.items) ? settlementInfo.items[0] : null
  if (!item) return '已完成'
  if (item.finalStatus === 'disputed' || (item.state && item.state.hasDispute)) return '活动异常待处理'
  if (item.finalStatus === 'cancelled' || item.participantDecision === 'cancelled') return '活动已取消'
  if (item.finalStatus === 'no_show' || item.authorDecision === 'no_show') return '未到场'
  if (item.finalStatus === 'completed') return '已完成'
  return '待履约确认'
}

function buildFlowSummary(post, settlementInfo, isAuthor, joined) {
  if (!post || post.status !== 'closed' || !settlementInfo) return ''
  if (settlementInfo.projectCancelled) {
    return isAuthor ? '项目已取消' : '活动已取消'
  }
  if (isAuthor) {
    if (hasAuthorSettlementWork(settlementInfo) || hasReviewTargets(settlementInfo)) return ''
    const reviewState = settlementInfo.reviewState || post.reviewState || {}
    const parts = ['履约已完成']
    if (Number(reviewState.reviewedCount || 0) > 0) {
      let reviewText = '已评价 ' + Number(reviewState.reviewedCount || 0) + ' 人'
      if (Number(reviewState.averageStars || 0) > 0) {
        reviewText += '，平均 ' + formatRatingDisplay(reviewState.averageStars || 0) + ' 星'
      }
      parts.push(reviewText)
    }
    return parts.join(' · ')
  }

  if (!joined) return ''
  if (hasParticipantSettlementWork(settlementInfo) || hasReviewTargets(settlementInfo)) return ''

  const parts = [buildParticipantResultText(settlementInfo)]
  const reviewState = settlementInfo.reviewState || post.reviewState || {}
  if (Number(reviewState.myStars || 0) > 0) {
    parts.push('我给了 ' + formatRatingDisplay(reviewState.myStars || 0) + ' 星')
  }
  return parts.join(' · ')
}

function buildStatusMeta(post, settlementInfo, isAuthor, isFull) {
  if (!post) {
    return { text: '加载中', tone: 'gray' }
  }
  // Moderation status takes priority over business status
  if (post.moderationStatus && post.moderationStatus !== 'approved') {
    if (post.moderationStatus === 'pending') return { text: '审核中', tone: 'orange' }
    if (post.moderationStatus === 'rejected') return { text: '已下架', tone: 'red' }
    if (post.moderationStatus === 'needs_revision') return { text: '待整改', tone: 'orange' }
    if (post.moderationStatus === 'manual_review') return { text: '人工审核中', tone: 'orange' }
    return { text: '审核中', tone: 'orange' }
  }
  if (settlementInfo && settlementInfo.projectCancelled) {
    return { text: isAuthor ? '项目已取消' : '活动已取消', tone: 'red' }
  }
  if (post.status === 'closed') {
    if (hasAuthorSettlementWork(settlementInfo) || hasParticipantSettlementWork(settlementInfo)) {
      return { text: '待履约确认', tone: 'blue' }
    }
    if (hasReviewTargets(settlementInfo)) {
      return { text: '待评分', tone: 'orange' }
    }
    return { text: '已完成', tone: 'gray' }
  }
  if (isFull && !isAuthor) {
    return { text: '名额已满', tone: 'orange' }
  }
  return { text: '进行中', tone: 'green' }
}

Page({
  data: {
    postId: '',
    post: null,
    participants: [],
    joined: false,
    isFull: false,
    isAuthor: false,
    timeText: '',
    loading: true,
    showLoginModal: false,
    pendingAuthAction: '',
    resumeAction: '',
    settlementInfo: null,
    statusMeta: { text: '加载中', tone: 'gray' },
    closedSummary: '',
    flowSummaryText: '',
    flowButtonText: '',
    showFlowButton: false,
    showChatButton: false,
    showJoinButton: false,
    showJoinedBadge: false,
    showFullBadge: false,
    showLoginFlowButton: false,
    // Moderation
    moderationBlocked: false,
    moderationReason: '',
    moderationRecordId: '',
    showAppealForm: false,
    appealText: '',
    appealSubmitting: false,
  },

  onLoad(options) {
    const id = options && options.id ? options.id : ''
    const resumeAction = options && options.resumeAction ? options.resumeAction : ''
    if (!id) {
      wx.showToast({ title: '缺少活动 ID', icon: 'none' })
      return
    }
    this._resumeHandled = false
    this.setData({ postId: id, resumeAction })
    this.loadAll()
  },

  onShow() {
    if (this.data.postId) {
      this.loadAll()
    }
  },

  currentUser() {
    return (getApp().globalData && getApp().globalData.currentUser) || null
  },

  loadAll() {
    const postId = this.data.postId
    if (!postId) return Promise.resolve()

    this.setData({ loading: true })
    return getPostDetail(postId)
      .then(({ post, participants }) => {
        const detailPost = decorateDetailPost(post)
        const currentUser = this.currentUser()
        const authorId = resolveAuthorId(detailPost)
        const isAuthor = !!(detailPost.viewerIsAuthor || (currentUser && currentUser.id === authorId))
        const joined = !!(detailPost.viewerJoined || (currentUser && participants.some((item) => item.id === currentUser.id)))
        const isFull = (detailPost.currentCount || 0) >= (detailPost.maxCount || 0)

        this.setData({
          post: detailPost,
          participants,
          joined,
          isFull,
          isAuthor,
          timeText: formatPostTime(detailPost.timeInfo) || '时间待确认',
        })

        if (detailPost.status === 'closed' && currentUser) {
          return getSettlement(postId).then((settlementInfo) => ({ settlementInfo }))
        }
        return { settlementInfo: null }
      })
      .then(({ settlementInfo }) => {
        const post = this.data.post
        const actionState = this.buildActionState(post, settlementInfo)
        const statusMeta = buildStatusMeta(post, settlementInfo, this.data.isAuthor, this.data.isFull)

        // Moderation state
        const moderationBlocked = !!(post && post.moderationStatus && post.moderationStatus !== 'approved')

        this.setData({
          settlementInfo,
          statusMeta,
          moderationBlocked,
          closedSummary: buildClosedSummary(post),
          flowSummaryText: buildFlowSummary(post, settlementInfo, this.data.isAuthor, this.data.joined),
          flowButtonText: actionState.flowButtonText,
          showFlowButton: actionState.showFlowButton,
          showChatButton: actionState.showChatButton,
          showJoinButton: actionState.showJoinButton,
          showJoinedBadge: actionState.showJoinedBadge,
          showFullBadge: actionState.showFullBadge,
          showLoginFlowButton: actionState.showLoginFlowButton,
          loading: false,
        })

        // If moderation blocked and author, fetch moderation detail for reason
        if (moderationBlocked && this.data.isAuthor && this.currentUser()) {
          getPostModeration(this.data.postId)
            .then((res) => {
              const record = res && res.moderation
              if (record) {
                this.setData({
                  moderationReason: record.decisionReason || '',
                  moderationRecordId: record.id || '',
                })
              }
            })
            .catch(() => { /* ignore — non-critical */ })
        }

        if (this.currentUser() && this.data.resumeAction && !this._resumeHandled) {
          this._resumeHandled = true
          setTimeout(() => this.executePendingAuthAction(this.data.resumeAction), 0)
        }
      })
      .catch((err) => {
        this.setData({ loading: false })
        wx.showToast({ title: (err && err.message) || '加载活动失败', icon: 'none' })
      })
  },

  buildResumeUrl(action) {
    const query = action ? '&resumeAction=' + encodeURIComponent(action) : ''
    return '/pages/detail/index?id=' + encodeURIComponent(this.data.postId) + query
  },

  openAuthModal(action) {
    this.setData({ showLoginModal: true, pendingAuthAction: action || '' })
  },

  onLoginModalClose() {
    this.setData({ showLoginModal: false, pendingAuthAction: '' })
  },

  onWechatLogin() {
    loginWithWechat()
      .then(() => {
        this.setData({ showLoginModal: false })
        wx.showToast({ title: '登录成功', icon: 'success' })
        return this.loadAll().then(() => this.executePendingAuthAction(this.data.pendingAuthAction))
      })
      .catch((err) => {
        wx.showToast({ title: (err && err.message) || '登录失败', icon: 'none' })
      })
  },

  onPasswordLogin() {
    const target = this.buildResumeUrl(this.data.pendingAuthAction)
    this.setData({ showLoginModal: false })
    gotoLoginPage(target)
  },

  onRegister() {
    const target = this.buildResumeUrl(this.data.pendingAuthAction)
    this.setData({ showLoginModal: false })
    gotoRegisterPage(target)
  },

  executePendingAuthAction(action) {
    const nextAction = String(action || '').trim()
    this.setData({ pendingAuthAction: '', resumeAction: '' })
    if (!nextAction) return
    if (nextAction === 'join') return this.performJoin()
    if (nextAction === 'chat') return this.openChatRoom()
    if (nextAction === 'settlement' || nextAction === 'flow') return this.openPostFlow()
  },

  performJoin() {
    const post = this.data.post
    if (!post || this.data.joined || this.data.isFull || this.data.isAuthor || post.status !== 'open') {
      return
    }
    if (!this.currentUser()) {
      this.openAuthModal('join')
      return
    }
    joinPost(post.id)
      .then(() => {
        wx.showToast({ title: '报名成功', icon: 'success' })
        return this.loadAll()
      })
      .catch((err) => {
        wx.showToast({ title: (err && err.message) || '报名失败', icon: 'none' })
      })
  },

  onJoinTap() {
    if (!this.currentUser()) {
      this.openAuthModal('join')
      return
    }
    this.performJoin()
  },

  onClosePostTap() {
    const post = this.data.post
    if (!post || !this.data.isAuthor || post.status !== 'open') return
    wx.showModal({
      title: '结束活动',
      content: '结束后将不再允许报名，并会记录结束时间。接下来会进入履约与评分流程，确认要结束吗？',
      success: (res) => {
        if (!res.confirm) return
        closePost(post.id)
          .then(() => {
            wx.showToast({ title: '活动已结束', icon: 'success' })
            return this.loadAll()
          })
          .catch((err) => {
            wx.showToast({ title: (err && err.message) || '结束活动失败', icon: 'none' })
          })
      },
    })
  },

  openChatRoom() {
    const post = this.data.post
    if (!post) return
    openPage('/pages/chat-room/index?id=' + encodeURIComponent(post.id) + '&title=' + encodeURIComponent(post.title || '群聊'))
  },

  onChatTap() {
    if (!this.currentUser()) {
      this.openAuthModal('chat')
      return
    }
    if (!this.data.showChatButton) return
    this.openChatRoom()
  },

  openSettlementFlow() {
    const post = this.data.post
    if (!post) return
    openPage('/pages/settlement/index?id=' + encodeURIComponent(post.id) + '&title=' + encodeURIComponent(post.title || '履约处理'))
  },

  openReviewFlow() {
    const post = this.data.post
    if (!post) return
    openPage('/pages/review/index?id=' + encodeURIComponent(post.id) + '&title=' + encodeURIComponent(post.title || '活动评分'))
  },

  openPostFlow() {
    const settlement = this.data.settlementInfo || {}
    if (hasReviewTargets(settlement) && !hasAuthorSettlementWork(settlement) && !hasParticipantSettlementWork(settlement)) {
      this.openReviewFlow()
      return
    }
    this.openSettlementFlow()
  },

  buildActionState(post, settlementInfo) {
    const isLoggedIn = !!this.currentUser()
    const joined = !!this.data.joined
    const isAuthor = !!this.data.isAuthor
    const isFull = !!this.data.isFull
    const actionState = {
      flowButtonText: '',
      showFlowButton: false,
      showChatButton: false,
      showJoinButton: false,
      showJoinedBadge: false,
      showFullBadge: false,
      showLoginFlowButton: false,
    }

    if (!post) return actionState

    // If post is not approved by moderation, disable all major actions
    if (post.moderationStatus && post.moderationStatus !== 'approved') {
      // Author can still see chat and edit (edit button handled in wxml)
      if (isAuthor) {
        actionState.showChatButton = true
      }
      return actionState
    }

    if (isAuthor) {
      actionState.showChatButton = true
      if (post.status === 'closed' && settlementInfo && !settlementInfo.projectCancelled) {
        if (hasAuthorSettlementWork(settlementInfo)) {
          actionState.showFlowButton = true
          actionState.flowButtonText = '管理履约'
        } else if (hasReviewTargets(settlementInfo)) {
          actionState.showFlowButton = true
          actionState.flowButtonText = '继续评分'
        }
      }
      return actionState
    }

    if (post.status === 'open') {
      actionState.showChatButton = joined
      actionState.showJoinButton = !joined && !isFull
      actionState.showJoinedBadge = joined
      actionState.showFullBadge = !joined && isFull
      return actionState
    }

    actionState.showChatButton = joined && isLoggedIn
    if (!isLoggedIn || !joined || !settlementInfo || settlementInfo.projectCancelled) {
      return actionState
    }
    if (hasParticipantSettlementWork(settlementInfo)) {
      actionState.showFlowButton = true
      actionState.flowButtonText = '履约确认'
      return actionState
    }
    if (hasReviewTargets(settlementInfo)) {
      actionState.showFlowButton = true
      actionState.flowButtonText = '去评分'
    }
    return actionState
  },

  onFlowTap() {
    if (!this.currentUser()) {
      this.openAuthModal('flow')
      return
    }
    if (!this.data.showFlowButton) return
    this.openPostFlow()
  },

  onEditTap() {
    const post = this.data.post
    if (!post) return
    openPage('/pages/edit/index?id=' + encodeURIComponent(post.id))
  },

  onReportTap() {
    if (!this.currentUser()) {
      this.openAuthModal('')
      return
    }
    const post = this.data.post
    if (!post) return
    wx.showActionSheet({
      itemList: ['内容不当', '虚假信息', '商业推广', '骚扰/攻击', '其他'],
      success: (res) => {
        const reasons = ['inappropriate_content', 'false_info', 'commercial', 'harassment', 'other']
        const reason = reasons[res.tapIndex] || 'other'
        reportPost(post.id, { description: reason })
          .then(() => wx.showToast({ title: '举报已提交', icon: 'success' }))
          .catch((err) => wx.showToast({ title: (err && err.message) || '举报失败', icon: 'none' }))
      },
    })
  },

  onAppealTap() {
    this.setData({ showAppealForm: true })
  },

  onAppealCancel() {
    this.setData({ showAppealForm: false, appealText: '' })
  },

  onAppealInput(e) {
    this.setData({ appealText: (e && e.detail && e.detail.value) || '' })
  },

  onAppealSubmit() {
    const recordId = this.data.moderationRecordId
    const text = (this.data.appealText || '').trim()
    if (!recordId || !text) {
      wx.showToast({ title: '请填写申诉原因', icon: 'none' })
      return
    }
    this.setData({ appealSubmitting: true })
    appealModeration(recordId, { description: text })
      .then(() => {
        wx.showToast({ title: '申诉已提交', icon: 'success' })
        this.setData({ showAppealForm: false, appealText: '', appealSubmitting: false })
      })
      .catch((err) => {
        this.setData({ appealSubmitting: false })
        wx.showToast({ title: (err && err.message) || '申诉失败', icon: 'none' })
      })
  },

  onAuthorTap() {
    const authorId = resolveAuthorId(this.data.post)
    if (!authorId) return
    openPage('/pages/user-home/index?id=' + encodeURIComponent(authorId))
  },
})
