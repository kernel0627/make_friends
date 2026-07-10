const { openPage } = require('../../utils/navigation')
const { getActivityHeatmap, getActivityPosts } = require('../../utils/calendarApi')
const { getLastLocation } = require('../../utils/location')

const WEEKDAY_HEADERS = ['日', '一', '二', '三', '四', '五', '六']
const LOOKAHEAD_DAYS = 30

function pad(value) {
  return String(value).padStart(2, '0')
}

function formatDate(date) {
  return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate())
}

function parseDate(raw) {
  const parts = String(raw || '').split('-').map((item) => Number(item))
  if (parts.length !== 3 || parts.some((item) => !Number.isFinite(item))) return null
  return new Date(parts[0], parts[1] - 1, parts[2])
}

function monthTitle(date) {
  if (!date) return ''
  return date.getFullYear() + '年' + (date.getMonth() + 1) + '月'
}

function selectedDateLabel(raw) {
  const date = parseDate(raw)
  if (!date) return '请选择日期'
  const today = new Date()
  const todayText = formatDate(today)
  if (raw === todayText) return '今天'
  return (date.getMonth() + 1) + '月' + date.getDate() + '日'
}

function decorateDay(day, selectedDate, todayText) {
  const date = parseDate(day.date)
  return Object.assign({}, day, {
    key: day.date,
    dayNumber: date ? date.getDate() : '',
    fireDisplay: day.fireText || '·',
    isToday: day.date === todayText,
    selected: day.date === selectedDate,
    empty: false,
  })
}

function buildMonthSections(days, selectedDate) {
  const todayText = formatDate(new Date())
  const sections = []
  let current = null

  days.forEach((day) => {
    const date = parseDate(day.date)
    if (!date) return
    const key = date.getFullYear() + '-' + pad(date.getMonth() + 1)
    if (!current || current.key !== key) {
      current = {
        key,
        title: monthTitle(date),
        cells: [],
      }
      const blanks = date.getDay()
      for (let i = 0; i < blanks; i++) {
        current.cells.push({ empty: true, key: key + '-blank-' + i })
      }
      sections.push(current)
    }
    current.cells.push(decorateDay(day, selectedDate, todayText))
  })

  return sections
}

function buildLocationParams() {
  const lastLocation = getLastLocation()
  if (!lastLocation) return {}
  return {
    userLat: lastLocation.latitude,
    userLng: lastLocation.longitude,
  }
}

Page({
  data: {
    weekdayHeaders: WEEKDAY_HEADERS,
    queryText: '',
    selectedDate: '',
    selectedDateLabel: '今天',
    monthSections: [],
    days: [],
    selectedPosts: [],
    loadingHeatmap: false,
    loadingPosts: false,
    errorText: '',
    postsErrorText: '',
  },

  onLoad() {
    const todayText = formatDate(new Date())
    this.setData({
      selectedDate: todayText,
      selectedDateLabel: selectedDateLabel(todayText),
    })
    this.loadHeatmap(true)
  },

  onPullDownRefresh() {
    this.loadHeatmap(false).finally(() => wx.stopPullDownRefresh())
  },

  buildQueryParams(extra) {
    const todayText = formatDate(new Date())
    return Object.assign({
      startDate: todayText,
      days: LOOKAHEAD_DAYS,
      query: (this.data.queryText || '').trim(),
    }, buildLocationParams(), extra || {})
  },

  loadHeatmap(resetSelection) {
    this._heatmapRequestId = (this._heatmapRequestId || 0) + 1
    const requestId = this._heatmapRequestId
    this.setData({
      loadingHeatmap: true,
      errorText: '',
    })

    return getActivityHeatmap(this.buildQueryParams())
      .then((res) => {
        if (requestId !== this._heatmapRequestId) return
        const days = Array.isArray(res.days) ? res.days : []
        let selectedDate = this.data.selectedDate || formatDate(new Date())
        if (resetSelection) {
          const firstHotDay = days.find((day) => day.highlighted || day.fireLevel > 0)
          selectedDate = firstHotDay ? firstHotDay.date : selectedDate
        }
        this.setData({
          days,
          selectedDate,
          selectedDateLabel: selectedDateLabel(selectedDate),
          monthSections: buildMonthSections(days, selectedDate),
          loadingHeatmap: false,
          errorText: '',
        })
        this.loadPostsForDate(selectedDate)
      })
      .catch((err) => {
        if (requestId !== this._heatmapRequestId) return
        this.setData({
          loadingHeatmap: false,
          errorText: (err && err.message) || '日历加载失败，请稍后重试',
          monthSections: [],
          days: [],
          selectedPosts: [],
        })
      })
  },

  loadPostsForDate(date) {
    if (!date) return Promise.resolve()
    this._postsRequestId = (this._postsRequestId || 0) + 1
    const requestId = this._postsRequestId
    this.setData({
      loadingPosts: true,
      postsErrorText: '',
    })

    return getActivityPosts(this.buildQueryParams({ date, limit: 20 }))
      .then((res) => {
        if (requestId !== this._postsRequestId) return
        this.setData({
          selectedPosts: Array.isArray(res.posts) ? res.posts : [],
          loadingPosts: false,
          postsErrorText: '',
        })
      })
      .catch((err) => {
        if (requestId !== this._postsRequestId) return
        this.setData({
          selectedPosts: [],
          loadingPosts: false,
          postsErrorText: (err && err.message) || '活动列表加载失败，请稍后重试',
        })
      })
  },

  onQueryInput(e) {
    this.setData({ queryText: (e.detail && e.detail.value) || '' })
  },

  onQueryConfirm() {
    this.loadHeatmap(true)
  },

  onSearchTap() {
    this.loadHeatmap(true)
  },

  onClearQueryTap() {
    this.setData({ queryText: '' })
    this.loadHeatmap(true)
  },

  onDateTap(e) {
    const date = e.currentTarget.dataset.date || ''
    if (!date || date === this.data.selectedDate) return
    this.setData({
      selectedDate: date,
      selectedDateLabel: selectedDateLabel(date),
      monthSections: buildMonthSections(this.data.days || [], date),
    })
    this.loadPostsForDate(date)
  },

  onPostCardTap(e) {
    const post = (e.detail && e.detail.post) || null
    if (!post || !post.id) return
    openPage('/pages/detail/index?id=' + post.id)
  },
})
