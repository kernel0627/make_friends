function pad(value) {
  return String(value).padStart(2, '0')
}

function formatDateTime(timestamp) {
  const value = Number(timestamp || 0)
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
}

function formatPostTime(timeInfo) {
  if (!timeInfo) return ''
  if (timeInfo.mode === 'range') {
    return '活动周期 ' + (timeInfo.days || 0) + ' 天'
  }
  if (timeInfo.mode === 'fixed' && timeInfo.fixedTime) {
    const date = new Date(timeInfo.fixedTime)
    if (Number.isNaN(date.getTime())) return ''
    return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
  }
  return ''
}

function formatPostTimeCompact(timeInfo) {
  if (!timeInfo) return ''
  if (timeInfo.mode === 'range') {
    return '活动周期 ' + (timeInfo.days || 0) + ' 天'
  }
  if (timeInfo.mode === 'fixed' && timeInfo.fixedTime) {
    const date = new Date(timeInfo.fixedTime)
    if (Number.isNaN(date.getTime())) return ''
    return pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
  }
  return ''
}

function truncate(text, maxLength) {
  const value = String(text || '').trim()
  if (!value) return ''
  if (!maxLength || value.length <= maxLength) return value
  return value.slice(0, maxLength) + '...'
}

function formatRatingDisplay(value) {
  return Number(value || 0).toFixed(1)
}

function closedAtText(post) {
  const text = formatDateTime(post && post.closedAt)
  return text ? ('结束于 ' + text) : ''
}

function hasFlowAction(post, role) {
  const settlement = (post && post.settlementState) || {}
  const review = (post && post.reviewState) || {}
  if (!post || post.status !== 'closed' || settlement.projectCancelled) return false
  if (role === 'author') {
    return !!settlement.canAuthorConfirm || !!review.canReview
  }
  return !!settlement.canParticipantConfirm || !!review.canReview
}

function statusFromHomePost(post, role) {
  const settlement = (post && post.settlementState) || {}
  const review = (post && post.reviewState) || {}

  if (settlement.projectCancelled || settlement.finalStatus === 'cancelled') {
    return { text: '已取消', tone: 'red' }
  }
  if (settlement.hasDispute || settlement.finalStatus === 'disputed') {
    return { text: '活动异常待处理', tone: 'orange' }
  }
  if (role === 'author') {
    if (settlement.canAuthorConfirm) {
      return { text: '待履约确认', tone: 'blue' }
    }
    if (review.canReview) {
      return { text: '待评分', tone: 'orange' }
    }
  } else {
    if (settlement.canParticipantConfirm) {
      return { text: '待履约确认', tone: 'blue' }
    }
    if (review.canReview) {
      return { text: '待评分', tone: 'orange' }
    }
    if (settlement.finalStatus === 'no_show') {
      return { text: '未到场', tone: 'gray' }
    }
  }
  if (post && post.status === 'closed') {
    return { text: '已完成', tone: 'gray' }
  }
  return { text: '进行中', tone: 'green' }
}

function buildScoreText(post, role) {
  const settlement = (post && post.settlementState) || {}
  const review = (post && post.reviewState) || {}
  const activity = (post && post.activityScore) || {}
  if (hasFlowAction(post, role) || settlement.projectCancelled || settlement.hasDispute) {
    return ''
  }

  const parts = []
  if (role === 'author') {
    if (review.reviewedCount > 0) {
      parts.push('已评价 ' + review.reviewedCount + ' 人，平均 ' + formatRatingDisplay(review.averageStars || 0) + ' 星')
    }
  } else if (review.myStars > 0) {
    parts.push('我给了 ' + formatRatingDisplay(review.myStars || 0) + ' 星')
  }

  if (activity.creditScore > 0 && post && post.status === 'closed' && !settlement.projectCancelled) {
    parts.push('本次信誉分 ' + activity.creditScore)
  }
  return parts.join(' · ')
}

function buildPreviewText(post, role) {
  const settlement = (post && post.settlementState) || {}
  const review = (post && post.reviewState) || {}
  const preview = (post && post.chatPreview) || {}

  if (settlement.projectCancelled || settlement.finalStatus === 'cancelled') {
    return role === 'author' ? '项目已取消，后续流程已终止' : '活动已取消，后续流程已终止'
  }
  if (settlement.hasDispute || settlement.finalStatus === 'disputed') {
    return '活动异常待处理，请留意后续结果'
  }
  if (role === 'author' && settlement.canAuthorConfirm) {
    const count = Number(settlement.pendingMemberCount || 0)
    return count > 0 ? ('还有 ' + count + ' 位成员待履约处理') : '活动已结束，请先完成履约处理'
  }
  if (role !== 'author' && settlement.canParticipantConfirm) {
    return '活动已结束，请先完成履约确认'
  }
  if (review.canReview) {
    return role === 'author' ? '履约已完成，继续给到场成员评分' : '履约已确认完成，去给发起人评分'
  }
  if (preview.latestMessage) {
    const senderPrefix = preview.latestMessageSender && preview.latestMessageSender.nickName
      ? (preview.latestMessageSender.nickName + '：')
      : ''
    return '群聊 · ' + truncate(senderPrefix + preview.latestMessage, 28)
  }
  if (post && post.status === 'closed') {
    return closedAtText(post) || '活动已结束'
  }
  return role === 'author' ? '活动进行中，留意报名和聊天消息' : '活动进行中，留意最新安排'
}

function buildAction(post, role) {
  const settlement = (post && post.settlementState) || {}
  const review = (post && post.reviewState) || {}
  const id = post && post.id ? post.id : ''
  const title = post && post.title ? post.title : '活动'
  if (!id || !post || post.status !== 'closed' || settlement.projectCancelled) {
    return null
  }

  if (role === 'author') {
    if (settlement.canAuthorConfirm) {
      return {
        text: '管理履约',
        route: '/pages/settlement/index?id=' + encodeURIComponent(id) + '&title=' + encodeURIComponent(title),
      }
    }
    if (review.canReview) {
      return {
        text: '继续评分',
        route: '/pages/review/index?id=' + encodeURIComponent(id) + '&title=' + encodeURIComponent(title),
      }
    }
    return null
  }

  if (settlement.canParticipantConfirm) {
    return {
      text: '履约确认',
      route: '/pages/settlement/index?id=' + encodeURIComponent(id) + '&title=' + encodeURIComponent(title),
    }
  }
  if (review.canReview) {
    return {
      text: '去评分',
      route: '/pages/review/index?id=' + encodeURIComponent(id) + '&title=' + encodeURIComponent(title),
    }
  }
  return null
}

function decorateHomePost(post, role) {
  const status = statusFromHomePost(post, role)
  return Object.assign({}, post, {
    timeText: formatPostTimeCompact(post && post.timeInfo),
    closedAtText: closedAtText(post),
    statusText: status.text,
    statusTone: status.tone,
    scoreText: buildScoreText(post, role),
    previewText: buildPreviewText(post, role),
    action: buildAction(post, role),
  })
}

module.exports = {
  formatDateTime,
  formatRatingDisplay,
  formatPostTime,
  formatPostTimeCompact,
  closedAtText,
  decorateHomePost,
  statusFromHomePost,
}
