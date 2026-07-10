Component({
  properties: {
    title: {
      type: String,
      value: '该功能需要登录后使用',
    },
    buttonText: {
      type: String,
      value: '去登录',
    },
  },

  methods: {
    onLoginTap() {
      this.triggerEvent('login')
    },
  },
})
