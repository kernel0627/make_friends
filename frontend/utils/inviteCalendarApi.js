const { request } = require('./http')
const { normalizeUser } = require('./postApi')

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

function normalizeInviteDay(day) {
  const source = day || {}
  return {
    date: source.date || '',
    candidateCount: safeNumber(source.candidateCount, 0),
    score: safeNumber(source.score, 0),
    fireLevel: safeNumber(source.fireLevel, 0),
    fireText: source.fireText || '',
    highlighted: !!source.highlighted,
    reason: source.reason || '',
  }
}

function normalizeInviteCandidate(item) {
  const source = item || {}
  return {
    user: normalizeUser(source.user || {}),
    matchScore: safeNumber(source.matchScore, 0),
    reasonTags: Array.isArray(source.reasonTags) ? source.reasonTags : [],
    reasonText: source.reasonText || '',
    selected: !!source.selected,
  }
}

function getInviteHeatmap(params) {
  return request({
    url: '/calendar/invite-heatmap' + buildQuery(params),
    method: 'GET',
  }).then((res) => ({
    startDate: (res && res.startDate) || '',
    endDate: (res && res.endDate) || '',
    days: Array.isArray(res && res.days) ? res.days.map(normalizeInviteDay) : [],
  }))
}

function getInviteCandidates(params) {
  return request({
    url: '/calendar/invite-candidates' + buildQuery(params),
    method: 'GET',
  }).then((res) => ({
    date: (res && res.date) || '',
    candidates: Array.isArray(res && res.candidates) ? res.candidates.map(normalizeInviteCandidate) : [],
  }))
}

module.exports = {
  getInviteHeatmap,
  getInviteCandidates,
}
