const { fetchChatMessages, sendChatMessage, connectChatRoom } = require('../../utils/chatApi')
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
    inputText: '',
    sending: false,
  },

  onLoad(options) {
    const id = options && options.id ? options.id : ''
    const title = decodeURIComponent((options && options.title) || '聊天室')
    wx.setNavigationBarTitle({ title })
    this.setData({ postId: id, title })
    if (ensurePageLogin(this)) {
      this.loadMessages()
      this.openLiveConnection()
    }
  },

  onShow() {
    if (ensurePageLogin(this) && this.data.postId) {
      this.loadMessages()
      this.openLiveConnection()
    }
  },

  onHide() {
    this.closeLiveConnection()
  },

  onUnload() {
    this.closeLiveConnection()
  },

  openLiveConnection() {
    if (this._chatConnection || !this.data.postId) return
    this._chatConnection = connectChatRoom(this.data.postId, {
      onMessage: (message) => this.appendMessage(message),
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
    if (existing.some((item) => item.id === message.id)) return

    const next = existing.concat(message).sort((a, b) => a.createdAt - b.createdAt)
    this.persistMessages(next)
    this.setData({
      messages: next,
      lastMsgId: next.length ? next[next.length - 1].id : '',
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

  loadMessages() {
    fetchChatMessages(this.data.postId).then((messages) => {
      this.persistMessages(messages)
      this.setData({
        messages,
        lastMsgId: messages.length ? messages[messages.length - 1].id : '',
      })
    }).catch((err) => {
      wx.showToast({ title: (err && err.message) || '加载消息失败', icon: 'none' })
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
        this.loadMessages()
        this.openLiveConnection()
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
})
