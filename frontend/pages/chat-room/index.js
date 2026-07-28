const { fetchChatMessages, sendChatMessage, connectChatRoom, mergeChatMessages } = require('../../utils/chatApi')
const { reportMessage } = require('../../utils/postApi')
const { ensurePageLogin, loginWithWechat, gotoLoginPage, gotoRegisterPage } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')
const { saveChatMessages } = require('../../utils/store')

Page({
  data: {
    isLoggedIn: false,
    showLoginModal: false,
    postId: '',
    messages: [],
    title: '',
    lastMsgId: '',
    lastCreatedAt: 0,
    inputText: '',
    sending: false,
  },

  onLoad(options) {
    const id = options && options.id ? options.id : ''
    const title = decodeURIComponent((options && options.title) || '聊天室')
    wx.setNavigationBarTitle({ title })
    this.setData({ postId: id, title })
    this._pageActive = true
    this.initializeChatRoom()
  },

  onShow() {
    this._pageActive = true
    this.initializeChatRoom()
  },

  onHide() {
    this._pageActive = false
    this.closeLiveConnection()
  },

  onUnload() {
    this._pageActive = false
    this.closeLiveConnection()
  },

  initializeChatRoom() {
    if (!ensurePageLogin(this) || !this.data.postId) return Promise.resolve()
    if (this._initializing) return this._initializing

    const incremental = Boolean(this._initialSyncDone)
    const options = incremental && this.data.lastCreatedAt > 0
      ? { sinceCreatedAt: this.data.lastCreatedAt, limit: 200 }
      : undefined
    const initializing = this.loadMessages(options)
      .then(() => {
        this._initialSyncDone = true
        if (this._pageActive) {
          this.openLiveConnection()
        }
      })
      .catch(() => {})
      .finally(() => {
        if (this._initializing === initializing) {
          this._initializing = null
        }
      })
    this._initializing = initializing
    return initializing
  },

  openLiveConnection() {
    if ((this._chatConnection && !this._chatConnection.closed) || !this.data.postId) return
    this._chatConnection = connectChatRoom(this.data.postId, {
      onMessage: (message) => this.appendMessage(message),
      onStatusChange: (status) => {
        if (status === 'unauthorized' && this._chatConnection) {
          this._chatConnection = null
        }
      },
      sinceCreatedAt: this.data.lastCreatedAt,
    })
  },

  closeLiveConnection() {
    if (!this._chatConnection) return
    this._chatConnection.close()
    this._chatConnection = null
  },

  // appendMessage is the single entry point for messages arriving from the
  // socket, polling, or our own send, so duplicates are dropped in one place.
  appendMessage(message) {
    if (!message || !message.id) return
    const existing = this.data.messages || []
    const next = mergeChatMessages(existing, [message])
    if (next.length === existing.length && existing.some((item) => item.id === message.id)) return
    this.persistMessages(next)
    this.setData({
      messages: next,
      lastMsgId: next.length ? next[next.length - 1].id : '',
      lastCreatedAt: next.length ? next[next.length - 1].createdAt : 0,
    })
  },

  persistMessages(messages) {
    const app = getApp()
    if (!app.globalData.chatMessages || typeof app.globalData.chatMessages !== 'object') {
      app.globalData.chatMessages = {}
    }
    app.globalData.chatMessages[this.data.postId] = messages.slice()
    saveChatMessages(app.globalData.chatMessages)
  },

  loadMessages(options) {
    return fetchChatMessages(this.data.postId, options).then((messages) => {
      const next = options && options.sinceCreatedAt !== undefined
        ? mergeChatMessages(this.data.messages || [], messages)
        : mergeChatMessages([], messages)
      this.persistMessages(next)
      this.setData({
        messages: next,
        lastMsgId: next.length ? next[next.length - 1].id : '',
        lastCreatedAt: next.length ? next[next.length - 1].createdAt : 0,
      })
      return next
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '加载消息失败', icon: 'none' })
      throw err
    })
  },

  onInputChange(e) {
    this.setData({ inputText: (e.detail && e.detail.value) || '' })
  },

  onSendTap() {
    if (!this.data.isLoggedIn) {
      this.onLoginTap()
      return
    }
    if (this.data.sending) return

    const content = (this.data.inputText || '').trim()
    if (!content) {
      wx.showToast({ title: '请输入消息内容', icon: 'none' })
      return
    }

    const app = getApp()
    const user = app.globalData.currentUser
    if (!user) {
      wx.showToast({ title: '请先登录', icon: 'none' })
      return
    }

    this.setData({ sending: true })
    sendChatMessage({
      postId: this.data.postId,
      content,
      sender: user,
      clientMsgId: 'client_' + Date.now(),
    }).then((msg) => {
      // The socket echoes our own message back; appendMessage dedupes by id.
      this.appendMessage(msg)
      this.setData({ inputText: '', sending: false })
    }).catch((err) => {
      this.setData({ sending: false })
      wx.showToast({ title: (err && err.message) || '发送失败，请重试', icon: 'none' })
    })
  },

  onGoDetail() {
    if (!this.data.postId) return
    openPage('/pages/detail/index?id=' + this.data.postId)
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
      if (ensurePageLogin(this)) {
        this.initializeChatRoom()
      }
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

  onMessageLongPress(e) {
    const msgId = e && e.currentTarget && e.currentTarget.dataset && e.currentTarget.dataset.id
    if (!msgId) return
    wx.showActionSheet({
      itemList: ['举报该消息'],
      success: (res) => {
        if (res.tapIndex === 0) {
          reportMessage(msgId, { description: 'inappropriate_content' })
            .then(() => wx.showToast({ title: '举报已提交', icon: 'success' }))
            .catch((err) => wx.showToast({ title: (err && err.message) || '举报失败', icon: 'none' }))
        }
      },
    })
  },
})
