const { request } = require('./http')

function normalizeChatMessage(msg, fallbackCreatedAt) {
  const createdAt = Number(msg && msg.createdAt) || fallbackCreatedAt || Date.now()
  return {
    id: (msg && msg.id) || ('msg_' + createdAt),
    postId: (msg && msg.postId) || '',
    sender: {
      id: (msg && msg.sender && msg.sender.id) || (msg && msg.senderId) || 'unknown',
      nickName: (msg && msg.sender && msg.sender.nickName) || '未知用户',
      avatarUrl: (msg && msg.sender && msg.sender.avatarUrl) || 'https://api.dicebear.com/7.x/avataaars/svg?seed=default',
    },
    content: (msg && msg.content) || '',
    time: formatChatTime(createdAt),
    createdAt,
  }
}

function fetchChatMessages(postId) {
  if (!postId) return Promise.resolve([])
  return request({
    url: '/chats/' + encodeURIComponent(postId) + '/messages',
    method: 'GET',
    noAuth: true,
  }).then((res) => {
    const list = Array.isArray(res && res.messages) ? res.messages : []
    return list
      .map((item, idx) => normalizeChatMessage(item, Date.now() + idx))
      .sort((a, b) => a.createdAt - b.createdAt)
  })
}

function sendChatMessage(params) {
  const postId = params && params.postId
  const content = ((params && params.content) || '').trim()
  const clientMsgId = (params && params.clientMsgId) || ''
  if (!postId) return Promise.reject(new Error('缺少活动信息'))
  if (!content) return Promise.reject(new Error('请输入消息内容'))

  return request({
    url: '/chats/' + encodeURIComponent(postId) + '/messages',
    method: 'POST',
    data: { content, clientMsgId },
  }).then((res) => normalizeChatMessage(res && res.message, Date.now()))
}

function formatChatTime(timestamp) {
  const date = new Date(timestamp)
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  return hh + ':' + mm
}

module.exports = {
  fetchChatMessages,
  sendChatMessage,
}