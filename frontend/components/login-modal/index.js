Component({
  properties: {
    visible: { type: Boolean, value: false },
    title: { type: String, value: '登录后继续' },
    tip: { type: String, value: '登录后就能参加活动、查看群聊和处理履约。' },
  },
  methods: {
    onMaskTap() { this.triggerEvent('close') },
    onCloseTap() { this.triggerEvent('close') },
    onWechatTap() { this.triggerEvent('wechat') },
    onPasswordTap() { this.triggerEvent('password') },
    onRegisterTap() { this.triggerEvent('register') },
  },
})