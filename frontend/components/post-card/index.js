function formatTimeText(timeInfo) {
  if (!timeInfo) return '时间待定'
  if (timeInfo.mode === 'range') {
    return '未来 ' + (timeInfo.days || 0) + ' 天内'
  }
  if (timeInfo.mode === 'fixed' && timeInfo.fixedTime) {
    const date = new Date(timeInfo.fixedTime)
    if (Number.isNaN(date.getTime())) return '时间待定'
    const pad = (value) => String(value).padStart(2, '0')
    return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes())
  }
  return '时间待定'
}

Component({
  properties: {
    post: { type: Object, value: null },
  },
  data: {
    timeText: '',
    descriptionText: '',
    statusText: '',
    statusTone: 'green',
    addressText: '',
    subCategoryText: '',
    authorName: '',
    authorAvatar: '',
  },
  methods: {
    syncDisplay(post) {
      const source = post || {}
      const description = String(source.description || '').trim()
      let statusText = '正在报名'
      let statusTone = 'green'

      if (source.status === 'closed') {
        statusText = '已结束'
        statusTone = 'gray'
      } else if ((source.currentCount || 0) >= (source.maxCount || 0)) {
        statusText = '名额已满'
        statusTone = 'orange'
      }

      const author = source.author || {}
      this.setData({
        timeText: formatTimeText(source.timeInfo),
        descriptionText: description.length > 44 ? (description.slice(0, 44) + '...') : description,
        statusText,
        statusTone,
        addressText: String(source.address || '').trim() || '地点待定',
        subCategoryText: String(source.subCategory || '').trim() || '活动',
        authorName: String(author.nickName || '').trim() || '匿名发起人',
        authorAvatar: String(author.avatarUrl || '').trim() || '/assets/icons/user.png',
      })
    },
    onTap() {
      const post = this.properties.post || null
      this.triggerEvent('cardtap', { post, postId: post && post.id ? post.id : '' }, { bubbles: true, composed: true })
    },
  },
  lifetimes: {
    attached() {
      this.syncDisplay(this.properties.post)
    },
  },
  observers: {
    post(post) {
      this.syncDisplay(post)
    },
  },
})
