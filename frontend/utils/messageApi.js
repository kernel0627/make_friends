const { request } = require('./http')
const { normalizePost, normalizeUser } = require('./postApi')

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

function normalizeInvitation(item) {
  const source = item || {}
  return {
    id: source.id || '',
    message: source.message || '',
    status: source.status || 'pending',
    respondedAt: safeNumber(source.respondedAt, 0),
    createdAt: safeNumber(source.createdAt, 0),
    updatedAt: safeNumber(source.updatedAt, 0),
    inviter: normalizeUser(source.inviter || {}),
    invitee: normalizeUser(source.invitee || {}),
    post: normalizePost(source.post || {}),
  }
}

function getInvitations() {
  return request({
    url: '/messages/invitations',
    method: 'GET',
  }).then((res) => ({
    invitations: Array.isArray(res && res.invitations) ? res.invitations.map(normalizeInvitation) : [],
  }))
}

function getSentInvitations() {
  return request({
    url: '/messages/sent-invitations',
    method: 'GET',
  }).then((res) => ({
    invitations: Array.isArray(res && res.invitations) ? res.invitations.map(normalizeInvitation) : [],
  }))
}

function acceptInvitation(invitationId) {
  return request({
    url: '/invitations/' + encodeURIComponent(invitationId || '') + '/accept',
    method: 'POST',
    data: {},
  }).then((res) => ({
    ok: !!(res && res.ok),
    post: normalizePost((res && res.post) || {}),
  }))
}

function rejectInvitation(invitationId) {
  return request({
    url: '/invitations/' + encodeURIComponent(invitationId || '') + '/reject',
    method: 'POST',
    data: {},
  })
}

function cancelInvitation(invitationId) {
  return request({
    url: '/invitations/' + encodeURIComponent(invitationId || '') + '/cancel',
    method: 'POST',
    data: {},
  })
}

function searchUsers(keyword, options) {
  return request({
    url: '/users/search' + buildQuery({
      q: keyword || '',
      limit: (options && options.limit) || 10,
    }),
    method: 'GET',
  }).then((res) => ({
    users: Array.isArray(res && res.users) ? res.users.map(normalizeUser) : [],
  }))
}

module.exports = {
  getInvitations,
  getSentInvitations,
  acceptInvitation,
  rejectInvitation,
  cancelInvitation,
  searchUsers,
  normalizeInvitation,
}
