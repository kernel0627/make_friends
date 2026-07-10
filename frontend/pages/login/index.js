const { passwordLogin, consumeLoginRedirect } = require('../../utils/auth')
const { openPage } = require('../../utils/navigation')

Page({
  data: {
    nickname: '',
    password: '',
    submitting: false,
  },

  onNicknameInput(e) {
    this.setData({ nickname: (e.detail && e.detail.value) || '' })
  },

  onPasswordInput(e) {
    this.setData({ password: (e.detail && e.detail.value) || '' })
  },

  onSubmit() {
    if (this.data.submitting) return

    const nickname = (this.data.nickname || '').trim()
    const password = (this.data.password || '').trim()
    if (!nickname || !password) {
      wx.showToast({ title: '请输入昵称和密码', icon: 'none' })
      return
    }

    this.setData({ submitting: true })
    passwordLogin(nickname, password)
      .then(() => {
        wx.showToast({ title: '登录成功', icon: 'success' })
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
        wx.showToast({ title: (err && err.message) || '登录失败', icon: 'none' })
      })
      .finally(() => {
        this.setData({ submitting: false })
      })
  },

  onGoRegister() {
    wx.navigateTo({ url: '/pages/register/index' })
  },
})
