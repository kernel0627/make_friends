const { request } = require('./http')

function safeNumber(value, fallback) {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

function normalizeUser(user) {
  const source = user || {}
  return {
    id: source.id || '',
    nickName: source.nickName || source.nickname || '未知用户',
    avatarUrl: source.avatarUrl || source.avatarURL || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
    creditScore: safeNumber(source.creditScore, 100),
    ratingScore: safeNumber(source.ratingScore, 5),
    createdAt: safeNumber(source.createdAt, 0),
    updatedAt: safeNumber(source.updatedAt, 0),
    role: source.role || 'user',
  }
}

function normalizeReviewState(reviewState) {
  const source = reviewState || {}
  return {
    canReview: !!source.canReview,
    pendingCount: safeNumber(source.pendingCount, 0),
    reviewedCount: safeNumber(source.reviewedCount, 0),
    myStars: safeNumber(source.myStars, 0),
    averageStars: safeNumber(source.averageStars, 0),
    statusText: source.statusText || '',
  }
}

function normalizeActivityScore(activityScore) {
  const source = activityScore || {}
  return {
    creditScore: safeNumber(source.creditScore, 0),
    ratingScore: safeNumber(source.ratingScore, 0),
    ratingCount: safeNumber(source.ratingCount, 0),
  }
}

function normalizeChatPreview(chatPreview) {
  const source = chatPreview || {}
  return {
    latestMessage: source.latestMessage || '',
    latestMessageAt: safeNumber(source.latestMessageAt, 0),
    latestMessageSender: source.latestMessageSender ? normalizeUser(source.latestMessageSender) : null,
  }
}

function normalizeSettlementState(state) {
  const source = state || {}
  return {
    canParticipantConfirm: !!source.canParticipantConfirm,
    canAuthorConfirm: !!source.canAuthorConfirm,
    canCancelAll: !!source.canCancelAll,
    canOpenFlow: !!source.canOpenFlow,
    projectCancelled: !!source.projectCancelled,
    finalStatus: source.finalStatus || '',
    hasDispute: !!source.hasDispute,
    participantDecision: source.participantDecision || '',
    authorDecision: source.authorDecision || '',
    reviewDeadlineAt: safeNumber(source.reviewDeadlineAt, 0),
    pendingMemberCount: safeNumber(source.pendingMemberCount, 0),
    flowLabel: source.flowLabel || '',
    myReviewStars: safeNumber(source.myReviewStars, 0),
    averageStars: safeNumber(source.averageStars, 0),
  }
}

function normalizePost(post) {
  const source = post || {}
  const authorId = source.authorId || (source.author && source.author.id) || ''
  const lat = safeNumber(source.lat, NaN)
  const lng = safeNumber(source.lng, NaN)
  const hasCoords = Number.isFinite(lat) && Number.isFinite(lng) && !(lat === 0 && lng === 0)

  return {
    id: source.id || '',
    authorId,
    title: source.title || '',
    description: source.description || '',
    category: source.category || '',
    subCategory: source.subCategory || '',
    timeInfo: {
      mode: source.timeMode || (source.timeInfo && source.timeInfo.mode) || 'range',
      days: safeNumber(source.timeDays || (source.timeInfo && source.timeInfo.days), 7),
      fixedTime: source.fixedTime || (source.timeInfo && source.timeInfo.fixedTime) || '',
    },
    address: source.address || '',
    coords: hasCoords ? { latitude: lat, longitude: lng } : null,
    maxCount: safeNumber(source.maxCount, 2),
    currentCount: safeNumber(source.currentCount, 1),
    status: source.status || 'open',
    createdAt: safeNumber(source.createdAt, 0),
    updatedAt: safeNumber(source.updatedAt, 0),
    closedAt: safeNumber(source.closedAt, 0),
    cancelledAt: safeNumber(source.cancelledAt, 0),
    viewerIsAuthor: !!source.viewerIsAuthor,
    viewerJoined: !!source.viewerJoined,
    recommendation: source.recommendation || null,
    author: normalizeUser(Object.assign({}, source.author || {}, { id: authorId })),
    reviewState: normalizeReviewState(source.reviewState),
    activityScore: normalizeActivityScore(source.activityScore),
    chatPreview: normalizeChatPreview(source.chatPreview),
    settlementState: normalizeSettlementState(source.settlementState),
  }
}

function normalizeSettlementItem(item) {
  const source = item || {}
  return {
    user: normalizeUser(source.user),
    relationStatus: source.relationStatus || '',
    participantDecision: source.participantDecision || '',
    authorDecision: source.authorDecision || '',
    finalStatus: source.finalStatus || '',
    participantNote: source.participantNote || '',
    authorNote: source.authorNote || '',
    participantConfirmedAt: safeNumber(source.participantConfirmedAt, 0),
    authorConfirmedAt: safeNumber(source.authorConfirmedAt, 0),
    settledAt: safeNumber(source.settledAt, 0),
    state: normalizeSettlementState(source.state),
  }
}

function settlementNeedsAuthorAttention(item) {
  if (!item) return false
  if (item.state && item.state.canAuthorConfirm) return true
  const status = String(item.finalStatus || '').trim()
  return status === '' || status === 'pending' || status === 'disputed'
}

function settlementNeedsParticipantAttention(item) {
  if (!item) return false
  if (item.state && item.state.canParticipantConfirm) return true
  const status = String(item.finalStatus || '').trim()
  return status === '' || status === 'pending' || status === 'disputed'
}

function inferSettlementStage(source, items, reviewTargets) {
  const stage = String(source.stage || '').trim()
  if (stage) return stage
  if (source.projectCancelled) return 'cancelled'
  if (Array.isArray(reviewTargets) && reviewTargets.length) return 'review'
  if (source.viewerIsAuthor) {
    return Array.isArray(items) && items.some(settlementNeedsAuthorAttention) ? 'settlement' : 'done'
  }
  return Array.isArray(items) && items.some(settlementNeedsParticipantAttention) ? 'settlement' : 'done'
}

function inferPendingMemberCount(source, items, viewerIsAuthor, stage) {
  const raw = safeNumber(source.pendingMemberCount, -1)
  if (raw >= 0) return raw
  if (!viewerIsAuthor || stage !== 'settlement' || !Array.isArray(items)) return 0
  return items.filter(settlementNeedsAuthorAttention).length
}

function buildSettlementFlowLabel(viewerIsAuthor, stage) {
  if (stage === 'settlement') return viewerIsAuthor ? '管理履约' : '履约确认'
  if (stage === 'review') return viewerIsAuthor ? '继续评分' : '去评分'
  if (stage === 'cancelled') return viewerIsAuthor ? '项目已取消' : '活动已取消'
  return '已完成'
}

function normalizeSettlement(settlement) {
  const source = settlement || {}
  const viewerIsAuthor = !!source.viewerIsAuthor
  const items = Array.isArray(source.items) ? source.items.map(normalizeSettlementItem) : []
  const reviewTargets = Array.isArray(source.reviewTargets)
    ? source.reviewTargets.map((item) => ({ user: normalizeUser(item.user) }))
    : []
  const reviewState = normalizeReviewState(source.reviewState)
  const stage = inferSettlementStage(source, items, reviewTargets)
  const pendingMemberCount = inferPendingMemberCount(source, items, viewerIsAuthor, stage)
  return {
    postId: source.postId || '',
    postTitle: source.postTitle || '',
    viewerIsAuthor,
    reviewDeadlineAt: safeNumber(source.reviewDeadlineAt, 0),
    projectCancelled: !!source.projectCancelled,
    canCancelAll: !!source.canCancelAll || (!!viewerIsAuthor && !source.projectCancelled && stage === 'settlement'),
    stage,
    flowLabel: source.flowLabel || buildSettlementFlowLabel(viewerIsAuthor, stage),
    pendingMemberCount,
    items,
    reviewTargets,
    reviewState,
  }
}

function buildQuery(params) {
  const pairs = []
  Object.keys(params || {}).forEach((key) => {
    const value = params[key]
    if (value === undefined || value === null || value === '') return
    pairs.push(encodeURIComponent(key) + '=' + encodeURIComponent(String(value)))
  })
  return pairs.length ? ('?' + pairs.join('&')) : ''
}

function listPosts(params) {
  const nextParams = Object.assign({}, params || {})
  if (nextParams.addressFilter && !nextParams.addressKeyword) {
    nextParams.addressKeyword = nextParams.addressFilter
  }
  delete nextParams.addressFilter
  return request({
    url: '/posts' + buildQuery(nextParams),
    method: 'GET',
  }).then((res) => ({
    posts: Array.isArray(res && res.posts) ? res.posts.map(normalizePost) : [],
    page: safeNumber(res && res.page, 1),
    pageSize: safeNumber(res && res.pageSize, 10),
    hasMore: !!(res && res.hasMore),
    nextPage: safeNumber(res && res.nextPage, 0),
    feedRequestId: (res && res.feedRequestId) || '',
  }))
}

function getPostDetail(postId) {
  return request({
    url: '/posts/' + encodeURIComponent(postId || ''),
    method: 'GET',
  }).then((res) => ({
    post: normalizePost((res && res.post) || {}),
    participants: Array.isArray(res && res.participants) ? res.participants.map(normalizeUser) : [],
  }))
}

function createPost(payload) {
  return request({ url: '/posts', method: 'POST', data: payload || {} })
    .then((res) => normalizePost((res && res.post) || {}))
}

function buildSmartHistoryPosts(posts) {
  return (Array.isArray(posts) ? posts : []).slice(0, 12).map((post) => ({
    title: post.title || '',
    description: post.description || '',
    category: post.category || '',
    subCategory: post.subCategory || '',
    timeInfo: post.timeInfo || { mode: 'range', days: 7, fixedTime: '' },
    address: post.address || '',
    coords: post.coords || null,
    maxCount: safeNumber(post.maxCount, 2),
    createdAt: safeNumber(post.createdAt, 0),
  }))
}

function buildSmartCurrentLocation(location) {
  if (!location) return null
  const latitude = safeNumber(location.latitude, NaN)
  const longitude = safeNumber(location.longitude, NaN)
  return {
    address: location.address || location.resolvedAddress || location.cachedAddress || '',
    latitude: Number.isFinite(latitude) ? latitude : 0,
    longitude: Number.isFinite(longitude) ? longitude : 0,
  }
}

function generateSmartPostDraft(input, options) {
  const opts = options || {}
  const history = opts.history || {}
  return request({
    url: '/posts/smart-draft',
    method: 'POST',
    timeout: 25000,
    data: {
      input: input || '',
      currentLocation: buildSmartCurrentLocation(opts.currentLocation),
      history: {
        initiatedPosts: buildSmartHistoryPosts(history.initiatedPosts),
        joinedPosts: buildSmartHistoryPosts(history.joinedPosts),
      },
    },
  }).then((res) => res || {})
}

function updatePost(postId, payload) {
  return request({ url: '/posts/' + encodeURIComponent(postId || ''), method: 'PUT', data: payload || {} })
    .then((res) => normalizePost((res && res.post) || {}))
}

function joinPost(postId) {
  return request({ url: '/posts/' + encodeURIComponent(postId || '') + '/join', method: 'POST', data: {} })
}

function cancelParticipation(postId) {
  return request({ url: '/posts/' + encodeURIComponent(postId || '') + '/participation/cancel', method: 'POST', data: {} })
}

function closePost(postId) {
  return request({ url: '/posts/' + encodeURIComponent(postId || '') + '/close', method: 'POST', data: {} })
    .then((res) => normalizePost((res && res.post) || {}))
}

function getSettlement(postId) {
  return request({ url: '/posts/' + encodeURIComponent(postId || '') + '/settlement', method: 'GET' })
    .then((res) => normalizeSettlement(res || {}))
}

function submitParticipantSettlement(postId, payload) {
  return request({
    url: '/posts/' + encodeURIComponent(postId || '') + '/settlement/participant',
    method: 'POST',
    data: payload || {},
  })
}

function submitAuthorSettlement(postId, payload) {
  return request({
    url: '/posts/' + encodeURIComponent(postId || '') + '/settlement/author',
    method: 'POST',
    data: payload || {},
  })
}

function cancelAllSettlement(postId) {
  return request({
    url: '/posts/' + encodeURIComponent(postId || '') + '/settlement/cancel-all',
    method: 'POST',
    data: {},
  })
}

function submitReviews(postId, items) {
  return request({
    url: '/posts/' + encodeURIComponent(postId || '') + '/reviews',
    method: 'POST',
    data: { items: Array.isArray(items) ? items : [] },
  })
}

function getUserHome(userId) {
  return request({
    url: '/users/' + encodeURIComponent(userId || '') + '/home',
    method: 'GET',
  }).then((res) => ({
    user: normalizeUser((res && res.user) || {}),
    initiatedPosts: Array.isArray(res && res.initiatedPosts) ? res.initiatedPosts.map(normalizePost) : [],
    joinedPosts: Array.isArray(res && res.joinedPosts) ? res.joinedPosts.map(normalizePost) : [],
    interestTags: Array.isArray(res && res.interestTags) ? res.interestTags : [],
  }))
}

function getCreditLedger(userId) {
  return request({
    url: '/users/' + encodeURIComponent(userId || '') + '/credit-ledger',
    method: 'GET',
  }).then((res) => ({
    items: Array.isArray(res && res.items) ? res.items : [],
  }))
}

function randomizeAvatar() {
  return request({ url: '/auth/avatar/random', method: 'POST', data: {} })
    .then((res) => normalizeUser((res && res.user) || {}))
}

function reportFeedExposures(payload) {
  return request({ url: '/recommendations/exposures', method: 'POST', data: payload || {}, noAuth: true, skipAuthRefresh: true })
}

function reportFeedClick(payload) {
  return request({ url: '/recommendations/click', method: 'POST', data: payload || {}, noAuth: true, skipAuthRefresh: true })
}

module.exports = {
  listPosts,
  getPostDetail,
  createPost,
  generateSmartPostDraft,
  updatePost,
  joinPost,
  cancelParticipation,
  closePost,
  getSettlement,
  submitParticipantSettlement,
  submitAuthorSettlement,
  cancelAllSettlement,
  submitReviews,
  getUserHome,
  getCreditLedger,
  randomizeAvatar,
  reportFeedExposures,
  reportFeedClick,
  normalizePost,
  normalizeUser,
}
