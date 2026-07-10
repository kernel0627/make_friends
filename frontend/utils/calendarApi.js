const { request } = require('./http')
const { normalizePost } = require('./postApi')

function safeNumber(value, fallback) {
  const number = Number(value)
  return Number.isFinite(number) ? number : fallback
}

function buildQuery(params) {
  const pairs = []
  Object.keys(params || {}).forEach((key) => {
    const value = params[key]
    if (value === undefined || value === null || value === '') return
    pairs.push(encodeURIComponent(key) + '=' + encodeURIComponent(String(value)))
  })
  return pairs.length ? ('?' + pairs.join('&')) : ''
}

function normalizeCalendarDay(day) {
  const source = day || {}
  return {
    date: source.date || '',
    activityCount: safeNumber(source.activityCount, 0),
    score: safeNumber(source.score, 0),
    fireLevel: safeNumber(source.fireLevel, 0),
    fireText: source.fireText || '',
    highlighted: !!source.highlighted,
    reason: source.reason || '',
  }
}

function getActivityHeatmap(params) {
  return request({
    url: '/calendar/activity-heatmap' + buildQuery(params),
    method: 'GET',
  }).then((res) => ({
    startDate: (res && res.startDate) || '',
    endDate: (res && res.endDate) || '',
    days: Array.isArray(res && res.days) ? res.days.map(normalizeCalendarDay) : [],
  }))
}

function getActivityPosts(params) {
  return request({
    url: '/calendar/activity-posts' + buildQuery(params),
    method: 'GET',
  }).then((res) => ({
    date: (res && res.date) || '',
    posts: Array.isArray(res && res.posts) ? res.posts.map(normalizePost) : [],
  }))
}

module.exports = {
  getActivityHeatmap,
  getActivityPosts,
}
