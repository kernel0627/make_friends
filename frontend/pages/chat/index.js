const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')
const { getUserHome } = require('../../utils/postApi')
const { getInvitations, getSentInvitations, acceptInvitation, rejectInvitation, cancelInvitation } = require('../../utils/messageApi')
const { statusFromHomePost } = require('../../utils/postPresentation')

function formatChatTime(timestamp) {
  const ts = Number(timestamp || 0)
  if (!ts) return '--'
  const d = new Date(ts)
  const now = new Date()
  const sameDay = d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate()
  const pad = (n) => String(n).padStart(2, '0')
  if (sameDay) return pad(d.getHours()) + ':' + pad(d.getMinutes())
  return pad(d.getMonth() + 1) + '-' + pad(d.getDate())
}

function formatPostTime(post) {
  const info = (post && post.timeInfo) || {}
  if (info.mode === 'fixed' && info.fixedTime) {
    const d = new Date(info.fixedTime)
    if (!Number.isNaN(d.getTime())) {
      const pad = (n) => String(n).padStart(2, '0')
      return (d.getMonth() + 1) + '月' + d.getDate() + '日 ' + pad(d.getHours()) + ':' + pad(d.getMinutes())
    }
  }
  return '未来 ' + (info.days || 7) + ' 天内'
}

function invitationStatusText(status) {
  switch (status) {
    case 'accepted': return '已接受'
    case 'rejected': return '已拒绝'
    case 'expired': return '已失效'
    case 'cancelled': return '已取消'
    default: return '待处理'
  }
}

function invitationStatusTone(status) {
  switch (status) {
    case 'accepted': return 'green'
    case 'rejected': return 'gray'
    case 'expired':
    case 'cancelled': return 'orange'
    default: return 'blue'
  }
}

function canRespondInvitation(status) {
  return status === 'pending'
}

function invitationProgressText(status) {
  switch (status) {
    case 'accepted': return '对方已接受'
    case 'rejected': return '对方已拒绝'
    case 'expired': return '邀请已失效'
    case 'cancelled': return '邀请已取消'
    default: return '等待对方回应'
  }
}

function canCancelInvitation(status) {
  return status === 'pending'
}

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    activeTab: 0,
    activeInvitationTab: 0,
    invitationsExpanded: false,
    invitationList: [],
    sentInvitationList: [],
    initiatedList: [],
    joinedList: [],
    invitationActionId: '',
  },

  onLoad() {
    const logged = ensurePageLogin(this)
    if (logged) this.refreshList()
  },

  onShow() {
    const logged = ensurePageLogin(this)
    if (logged) this.refreshList()
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
      const logged = ensurePageLogin(this)
      if (logged) this.refreshList()
      wx.showToast({ title: '登录成功', icon: 'success' })
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '登录失败，请重试', icon: 'none' })
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

  refreshList() {
    const app = getApp()
    const currentUser = app.globalData.currentUser
    if (!currentUser || !currentUser.id) {
      this.setData({ invitationList: [], sentInvitationList: [], initiatedList: [], joinedList: [] })
      return
    }

    Promise.all([getUserHome(currentUser.id), getInvitations(), getSentInvitations()]).then(([home, messages, sentMessages]) => {
      this.setData({
        invitationList: this.buildInvitationItems(messages.invitations || []),
        sentInvitationList: this.buildSentInvitationItems(sentMessages.invitations || []),
        initiatedList: this.buildChatItems(home.initiatedPosts || [], 'author'),
        joinedList: this.buildChatItems(home.joinedPosts || [], 'participant'),
      })
    }).catch((err) => {
      this.setData({ invitationList: [], sentInvitationList: [], initiatedList: [], joinedList: [] })
      wx.showToast({ title: (err && err.message) || '加载消息失败', icon: 'none' })
    })
  },

  buildInvitationItems(invitations) {
    return (invitations || []).map((item) => {
      const post = item.post || {}
      const inviter = item.inviter || {}
      const status = item.status || 'pending'
      return {
        id: item.id,
        postId: post.id,
        postTitle: post.title || '未命名活动',
        inviterName: inviter.nickName || '发起人',
        inviterAvatar: inviter.avatarUrl || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
        message: item.message || '我觉得你可能会对这个活动感兴趣，一起来吗？',
        timeText: formatPostTime(post),
        address: post.address || '地点待确认',
        countText: (post.currentCount || 0) + ' / ' + (post.maxCount || 0) + ' 人',
        status,
        statusText: invitationStatusText(status),
        statusTone: invitationStatusTone(status),
        canRespond: canRespondInvitation(status),
        createdAt: item.createdAt || 0,
        createdText: formatChatTime(item.createdAt),
      }
    }).sort((a, b) => {
      if (a.canRespond !== b.canRespond) return a.canRespond ? -1 : 1
      return b.createdAt - a.createdAt
    })
  },

  buildSentInvitationItems(invitations) {
    return (invitations || []).map((item) => {
      const post = item.post || {}
      const invitee = item.invitee || {}
      const status = item.status || 'pending'
      return {
        id: item.id,
        postId: post.id,
        postTitle: post.title || '未命名活动',
        inviteeName: invitee.nickName || '被邀请用户',
        inviteeAvatar: invitee.avatarUrl || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
        message: item.message || '我觉得你可能会对这个活动感兴趣，一起来吗？',
        timeText: formatPostTime(post),
        address: post.address || '地点待确认',
        countText: (post.currentCount || 0) + ' / ' + (post.maxCount || 0) + ' 人',
        status,
        statusText: invitationStatusText(status),
        statusTone: invitationStatusTone(status),
        progressText: invitationProgressText(status),
        canCancel: canCancelInvitation(status),
        createdAt: item.createdAt || 0,
        createdText: formatChatTime(item.createdAt),
        respondedText: item.respondedAt ? formatChatTime(item.respondedAt) : '',
      }
    }).sort((a, b) => {
      const aPending = a.status === 'pending'
      const bPending = b.status === 'pending'
      if (aPending !== bPending) return aPending ? -1 : 1
      return b.createdAt - a.createdAt
    })
  },

  buildChatItems(posts, role) {
    const items = (posts || []).map((post) => {
      const preview = post.chatPreview || {}
      const sender = preview.latestMessageSender || null
      const lastTimestamp = preview.latestMessageAt || post.updatedAt || post.createdAt || 0
      const status = statusFromHomePost(post, role)
      return {
        id: post.id,
        title: post.title || '未命名活动',
        avatarUrl: (sender && sender.avatarUrl) || (post.author && post.author.avatarUrl) || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
        lastMessage: preview.latestMessage || '进入群聊后就能开始交流',
        senderName: sender ? sender.nickName : '',
        lastTime: formatChatTime(lastTimestamp),
        lastTimestamp,
        statusText: status.text,
        statusTone: status.tone,
      }
    })

    items.sort((a, b) => b.lastTimestamp - a.lastTimestamp)
    return items
  },

  onTabTap(e) {
    const idx = Number(e.currentTarget.dataset.index || 0)
    this.setData({ activeTab: idx })
  },

  onInvitationToggle() {
    this.setData({ invitationsExpanded: !this.data.invitationsExpanded })
  },

  onInvitationTabTap(e) {
    const idx = Number(e.currentTarget.dataset.index || 0)
    this.setData({ activeInvitationTab: idx })
  },

  onItemTap(e) {
    const id = e.currentTarget.dataset.id
    const title = e.currentTarget.dataset.title
    openPage('/pages/chat-room/index?id=' + id + '&title=' + encodeURIComponent(title))
  },

  onInvitationDetail(e) {
    const postId = e.currentTarget.dataset.postId
    if (!postId) return
    openPage('/pages/detail/index?id=' + postId)
  },

  onInvitationAccept(e) {
    const id = e.currentTarget.dataset.id
    if (!id || this.data.invitationActionId) return
    this.setData({ invitationActionId: id })
    acceptInvitation(id).then(() => {
      wx.showToast({ title: '已接受邀请', icon: 'success' })
      this.refreshList()
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '接受邀请失败', icon: 'none' })
      this.refreshList()
    }).finally(() => {
      this.setData({ invitationActionId: '' })
    })
  },

  onInvitationReject(e) {
    const id = e.currentTarget.dataset.id
    if (!id || this.data.invitationActionId) return
    this.setData({ invitationActionId: id })
    rejectInvitation(id).then(() => {
      wx.showToast({ title: '已拒绝邀请', icon: 'success' })
      this.refreshList()
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '拒绝邀请失败', icon: 'none' })
      this.refreshList()
    }).finally(() => {
      this.setData({ invitationActionId: '' })
    })
  },

  onInvitationCancel(e) {
    const id = e.currentTarget.dataset.id
    if (!id || this.data.invitationActionId) return
    this.setData({ invitationActionId: id })
    cancelInvitation(id).then(() => {
      wx.showToast({ title: '已撤回邀请', icon: 'success' })
      this.refreshList()
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '撤回邀请失败', icon: 'none' })
      this.refreshList()
    }).finally(() => {
      this.setData({ invitationActionId: '' })
    })
  },
})
