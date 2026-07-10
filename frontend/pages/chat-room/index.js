const { fetchChatMessages, sendChatMessage } = require('../../utils/chatApi')
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
    }
  },

  onShow() {
    if (ensurePageLogin(this) && this.data.postId) {
      this.loadMessages()
    }
  },

  loadMessages() {
    fetchChatMessages(this.data.postId).then((messages) => {
      const app = getApp()
      if (!app.globalData.chatMessages || typeof app.globalData.chatMessages !== 'object') {
        app.globalData.chatMessages = {}
      }
      app.globalData.chatMessages[this.data.postId] = messages.slice()
      saveChatMessages(app.globalData.chatMessages)
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
      const next = this.data.messages.concat(msg)
      const app = getApp()
      if (!app.globalData.chatMessages || typeof app.globalData.chatMessages !== 'object') {
        app.globalData.chatMessages = {}
      }
      app.globalData.chatMessages[this.data.postId] = next.slice()
      saveChatMessages(app.globalData.chatMessages)
      this.setData({
        messages: next,
        inputText: '',
        lastMsgId: msg.id,
        sending: false,
      })
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
      if (ensurePageLogin(this)) this.loadMessages()
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
