const { getInviteHeatmap, getInviteCandidates } = require('../../utils/inviteCalendarApi')

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
  const todayText = formatDate(new Date())
  if (raw === todayText) return '今天'
  return (date.getMonth() + 1) + '月' + date.getDate() + '日'
}

function detectPeriod(text) {
  const value = String(text || '').toLowerCase()
  if (value.includes('上午') || value.includes('早上') || value.includes('早晨') || value.includes('morning')) return 'morning'
  if (value.includes('下午') || value.includes('午后') || value.includes('afternoon')) return 'afternoon'
  if (value.includes('夜间') || value.includes('深夜') || value.includes('夜里') || value.includes('night')) return 'night'
  if (value.includes('晚上') || value.includes('今晚') || value.includes('晚间') || value.includes('evening')) return 'evening'
  return ''
}

function periodLabel(period) {
  switch (period) {
    case 'morning': return '上午'
    case 'afternoon': return '下午'
    case 'night': return '夜间'
    default: return '晚上'
  }
}

function clockForPeriod(period) {
  switch (period) {
    case 'morning': return '09:00'
    case 'afternoon': return '15:00'
    case 'night': return '21:00'
    default: return '19:00'
  }
}

function inferPeriodFromClock(clock) {
  const hour = Number(String(clock || '').slice(0, 2))
  if (!Number.isFinite(hour)) return ''
  if (hour >= 5 && hour < 12) return 'morning'
  if (hour >= 12 && hour < 18) return 'afternoon'
  if (hour >= 18 && hour < 21) return 'evening'
  return 'night'
}

function inferContextPeriod(context) {
  if (context && context.selectedClock) return inferPeriodFromClock(context.selectedClock)
  if (context && context.fixedTime) {
    const date = new Date(context.fixedTime)
    if (!Number.isNaN(date.getTime())) return inferPeriodFromClock(pad(date.getHours()) + ':' + pad(date.getMinutes()))
  }
  return ''
}

function formatRating(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(2) : '暂无'
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
  ;(days || []).forEach((day) => {
    const date = parseDate(day.date)
    if (!date) return
    const key = date.getFullYear() + '-' + pad(date.getMonth() + 1)
    if (!current || current.key !== key) {
      current = { key, title: monthTitle(date), cells: [] }
      for (let i = 0; i < date.getDay(); i++) {
        current.cells.push({ empty: true, key: key + '-blank-' + i })
      }
      sections.push(current)
    }
    current.cells.push(decorateDay(day, selectedDate, todayText))
  })
  return sections
}

function normalizeInvitee(user) {
  const source = user || {}
  return {
    id: source.id || '',
    nickName: source.nickName || '未知用户',
    avatarUrl: source.avatarUrl || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
    creditScore: Number(source.creditScore || 100),
    ratingScore: Number(source.ratingScore || 5),
    ratingText: formatRating(source.ratingScore),
  }
}

function mergeInvitees(current, incoming) {
  const map = {}
  ;(current || []).forEach((user) => {
    const normalized = normalizeInvitee(user)
    if (normalized.id) map[normalized.id] = normalized
  })
  ;(incoming || []).forEach((user) => {
    const normalized = normalizeInvitee(user)
    if (normalized.id) map[normalized.id] = normalized
  })
  return Object.keys(map).map((id) => map[id])
}

Page({
  data: {
    weekdayHeaders: WEEKDAY_HEADERS,
    queryText: '',
    context: {},
    selectedDate: '',
    selectedDateLabel: '今天',
    selectedPeriod: 'evening',
    monthSections: [],
    days: [],
    candidates: [],
    selectedInvitees: [],
    loadingHeatmap: false,
    loadingCandidates: false,
    errorText: '',
    candidatesErrorText: '',
  },

  onLoad() {
    const todayText = formatDate(new Date())
    this.setData({
      selectedDate: todayText,
      selectedDateLabel: selectedDateLabel(todayText),
    })
    this._receivedInit = false
    const eventChannel = this.getOpenerEventChannel && this.getOpenerEventChannel()
    if (eventChannel && eventChannel.on) {
      eventChannel.on('initInviteCalendar', (payload) => {
        this._receivedInit = true
        this.initFromSnapshot((payload && payload.snapshot) || {})
      })
    }
    setTimeout(() => {
      if (!this._receivedInit) this.initFromSnapshot({})
    }, 0)
  },

  initFromSnapshot(snapshot) {
    const context = snapshot || {}
    const period = detectPeriod(context.title || '') || inferContextPeriod(context) || 'evening'
    this.setData({
      context,
      selectedInvitees: mergeInvitees([], context.selectedInvitees || []),
      selectedPeriod: period,
    })
    this.loadHeatmap(true)
  },

  buildParams(extra) {
    const context = this.data.context || {}
    const coords = context.locationCoords || null
    const queryText = (this.data.queryText || '').trim()
    const period = detectPeriod(queryText) || this.data.selectedPeriod || inferContextPeriod(context) || 'evening'
    const params = {
      startDate: formatDate(new Date()),
      days: LOOKAHEAD_DAYS,
      query: queryText,
      category: context.category || '',
      subCategory: context.subCategory || '',
      address: context.locationText || '',
      period,
      maxCount: context.maxCount || 2,
    }
    if (coords && coords.latitude !== undefined && coords.longitude !== undefined) {
      params.lat = coords.latitude
      params.lng = coords.longitude
    }
    return Object.assign(params, extra || {})
  },

  selectedIdText() {
    return (this.data.selectedInvitees || []).map((user) => user.id).filter(Boolean).join(',')
  },

  loadHeatmap(resetSelection) {
    this._heatmapRequestId = (this._heatmapRequestId || 0) + 1
    const requestId = this._heatmapRequestId
    this.setData({ loadingHeatmap: true, errorText: '' })
    return getInviteHeatmap(this.buildParams())
      .then((res) => {
        if (requestId !== this._heatmapRequestId) return
        const days = Array.isArray(res.days) ? res.days : []
        let selectedDate = this.data.selectedDate || formatDate(new Date())
        if (resetSelection) {
          const firstHotDay = days.find((day) => day.highlighted || day.candidateCount > 0)
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
        this.loadCandidatesForDate(selectedDate)
      })
      .catch((err) => {
        if (requestId !== this._heatmapRequestId) return
        this.setData({
          loadingHeatmap: false,
          errorText: (err && err.message) || '日历找人加载失败，请稍后重试',
          days: [],
          monthSections: [],
          candidates: [],
        })
      })
  },

  loadCandidatesForDate(date) {
    if (!date) return Promise.resolve()
    this._candidateRequestId = (this._candidateRequestId || 0) + 1
    const requestId = this._candidateRequestId
    this.setData({ loadingCandidates: true, candidatesErrorText: '' })
    return getInviteCandidates(this.buildParams({
      date,
      limit: 30,
      selectedIds: this.selectedIdText(),
    })).then((res) => {
      if (requestId !== this._candidateRequestId) return
      const selectedMap = this.data.selectedInvitees.reduce((acc, user) => {
        acc[user.id] = true
        return acc
      }, {})
      const candidates = (res.candidates || []).map((item) => Object.assign({}, item, {
        id: item.user.id,
        selected: !!selectedMap[item.user.id] || item.selected,
        ratingText: formatRating(item.user.ratingScore),
      }))
      this.setData({ candidates, loadingCandidates: false, candidatesErrorText: '' })
    }).catch((err) => {
      if (requestId !== this._candidateRequestId) return
      this.setData({
        candidates: [],
        loadingCandidates: false,
        candidatesErrorText: (err && err.message) || '候选成员加载失败',
      })
    })
  },

  onQueryInput(e) {
    this.setData({ queryText: (e.detail && e.detail.value) || '' })
  },

  onSearchTap() {
    const period = detectPeriod(this.data.queryText)
    this.setData({ selectedPeriod: period || this.data.selectedPeriod || 'evening' })
    this.loadHeatmap(true)
  },

  onQueryConfirm() {
    this.onSearchTap()
  },

  onDateTap(e) {
    const date = e.currentTarget.dataset.date || ''
    if (!date || date === this.data.selectedDate) return
    this.setData({
      selectedDate: date,
      selectedDateLabel: selectedDateLabel(date),
      monthSections: buildMonthSections(this.data.days || [], date),
    })
    this.loadCandidatesForDate(date)
  },

  onCandidateToggle(e) {
    const userId = e.currentTarget.dataset.id
    if (!userId) return
    const selected = this.data.selectedInvitees.slice()
    const index = selected.findIndex((user) => user.id === userId)
    if (index >= 0) {
      selected.splice(index, 1)
    } else {
      const candidate = (this.data.candidates || []).find((item) => item.user.id === userId)
      if (candidate) selected.push(normalizeInvitee(candidate.user))
    }
    const selectedMap = selected.reduce((acc, user) => {
      acc[user.id] = true
      return acc
    }, {})
    this.setData({
      selectedInvitees: selected,
      candidates: (this.data.candidates || []).map((item) => Object.assign({}, item, {
        selected: !!selectedMap[item.user.id],
      })),
    })
  },

  onApplyTap() {
    const selectedDate = this.data.selectedDate
    if (!selectedDate) {
      wx.showToast({ title: '请选择日期', icon: 'none' })
      return
    }
    const period = detectPeriod(this.data.queryText) || this.data.selectedPeriod || 'evening'
    const selectedClock = clockForPeriod(period)
    const fixedDate = new Date((selectedDate + ' ' + selectedClock).replace(/-/g, '/') + ':00')
    if (!Number.isFinite(fixedDate.getTime()) || fixedDate.getTime() <= Date.now()) {
      wx.showToast({ title: '所选日期时间已过，请换一天', icon: 'none' })
      return
    }
    const eventChannel = this.getOpenerEventChannel && this.getOpenerEventChannel()
    if (eventChannel && eventChannel.emit) {
      eventChannel.emit('inviteCalendarApply', {
        selectedDate,
        selectedClock,
        period,
        periodText: periodLabel(period),
        selectedInvitees: this.data.selectedInvitees,
      })
    }
    wx.navigateBack()
  },
})
