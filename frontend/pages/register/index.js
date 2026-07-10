const { registerWithPassword, consumeLoginRedirect } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')

Page({
  data: {
    nickname: '',
    password: '',
    confirmPassword: '',
    submitting: false,
  },

  onNicknameInput(e) {
    this.setData({ nickname: (e.detail && e.detail.value) || '' })
  },

  onPasswordInput(e) {
    this.setData({ password: (e.detail && e.detail.value) || '' })
  },

  onConfirmInput(e) {
    this.setData({ confirmPassword: (e.detail && e.detail.value) || '' })
  },

  onSubmit() {
    if (this.data.submitting) return

    const nickname = (this.data.nickname || '').trim()
    const password = (this.data.password || '').trim()
    const confirm = (this.data.confirmPassword || '').trim()

    if (!nickname || !password) {
      wx.showToast({ title: '请输入昵称和密码', icon: 'none' })
      return
    }
    if (password.length < 6) {
      wx.showToast({ title: '密码至少 6 位', icon: 'none' })
      return
    }
    if (password !== confirm) {
      wx.showToast({ title: '两次密码不一致', icon: 'none' })
      return
    }

    this.setData({ submitting: true })
    registerWithPassword(nickname, password)
      .then(() => {
        wx.showToast({ title: '注册成功', icon: 'success' })
        const target = consumeLoginRedirect()
        setTimeout(function() {
          if (target) {
            openPage(target)
            return
          }
          wx.switchTab({ url: '/pages/profile/index' })
        }, 300)
      })
      .catch((err) => {
        wx.showToast({ title: (err && err.message) || '注册失败', icon: 'none' })
      })
      .finally(() => {
        this.setData({ submitting: false })
      })
  },

  onGoLogin() {
    wx.navigateTo({ url: '/pages/login/index' })
  },
})
