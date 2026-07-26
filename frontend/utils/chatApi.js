const { request, getAccessToken } = require('./http')
const { WS_BASE_URL } = require('./config')

const RECONNECT_BASE_DELAY = 1000
const RECONNECT_MAX_DELAY = 15000
const POLL_INTERVAL = 5000

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

function mergeChatMessages(existing, incoming) {
  const byId = {}
  ;(existing || []).concat(incoming || []).forEach((item) => {
    if (!item || !item.id) return
    byId[item.id] = item
  })
  return Object.keys(byId)
    .map((id) => byId[id])
    .sort((a, b) => {
      const byCreatedAt = Number(a.createdAt || 0) - Number(b.createdAt || 0)
      return byCreatedAt || String(a.id).localeCompare(String(b.id))
    })
}

function fetchChatMessages(postId, options) {
  if (!postId) return Promise.resolve([])
  const data = {}
  const sinceCreatedAt = Number(options && options.sinceCreatedAt)
  if (Number.isFinite(sinceCreatedAt) && sinceCreatedAt >= 0) {
    data.sinceCreatedAt = sinceCreatedAt
    const requestedLimit = Number(options && options.limit)
    if (Number.isFinite(requestedLimit) && requestedLimit > 0) {
      data.limit = Math.min(Math.floor(requestedLimit), 500)
    }
  }
  return request({
    url: '/chats/' + encodeURIComponent(postId) + '/messages',
    method: 'GET',
    data,
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

// connectChatRoom opens a live connection for a post's chat room and calls
// handlers.onMessage for each incoming message. The server requires Redis for
// websockets, so when the socket cannot be established this falls back to
// polling the REST endpoint rather than leaving the room silent.
//
// Returns a handle with close(); callers must invoke it on page unload.
function connectChatRoom(postId, handlers) {
  const onMessage = (handlers && handlers.onMessage) || function() {}
  const onStatusChange = (handlers && handlers.onStatusChange) || function() {}

  let closed = false
  let socket = null
  let reconnectTimer = null
  let pollTimer = null
  let attempt = 0
  let latestCreatedAt = Number(handlers && handlers.sinceCreatedAt) || 0

  function clearTimers() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function isUnauthorized(value) {
    if (!value) return false
    if (Number(value.statusCode) === 401 || Number(value.code) === 401) return true
    const text = String(value.errMsg || value.reason || value.message || '')
    return /(?:^|\D)401(?:\D|$)|unauthori[sz]ed/i.test(text)
  }

  function stopUnauthorized() {
    if (closed) return
    closed = true
    clearTimers()
    if (socket) {
      try { socket.close({}) } catch (e) {}
      socket = null
    }
    onStatusChange('unauthorized')
  }

  function deliver(message) {
    if (!message || !message.id) return
    latestCreatedAt = Math.max(latestCreatedAt, Number(message.createdAt) || 0)
    onMessage(message)
  }

  function startPolling() {
    if (closed || pollTimer) return
    onStatusChange('polling')
    pollTimer = setInterval(() => {
      fetchChatMessages(postId, {
        sinceCreatedAt: latestCreatedAt,
        limit: 200,
      })
        .then((messages) => {
          if (closed) return
          messages.forEach(deliver)
        })
        .catch((err) => {
          if (isUnauthorized(err)) {
            stopUnauthorized()
          }
        })
    }, POLL_INTERVAL)
  }

  function scheduleReconnect() {
    if (closed || reconnectTimer) return
    // Exponential backoff, capped. Polling covers the gap meanwhile.
    const delay = Math.min(RECONNECT_BASE_DELAY * Math.pow(2, attempt), RECONNECT_MAX_DELAY)
    attempt += 1
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      open()
    }, delay)
  }

  function open() {
    if (closed) return
    const token = getAccessToken()
    if (!token) {
      stopUnauthorized()
      return
    }

    const url = WS_BASE_URL + '/ws/chat?postId=' + encodeURIComponent(postId)
    let task = null
    try {
      task = wx.connectSocket({
        url,
        header: { Authorization: 'Bearer ' + token },
      })
    } catch (e) {
      startPolling()
      scheduleReconnect()
      return
    }
    if (!task) {
      startPolling()
      scheduleReconnect()
      return
    }
    socket = task

    task.onOpen(() => {
      if (closed) {
        try { task.close({}) } catch (e) {}
        return
      }
      attempt = 0
      if (pollTimer) {
        clearInterval(pollTimer)
        pollTimer = null
      }
      onStatusChange('online')
    })

    task.onMessage((res) => {
      if (closed) return
      let payload = null
      try {
        payload = JSON.parse((res && res.data) || '{}')
      } catch (e) {
        return
      }
      if (!payload || payload.type !== 'chat_message' || !payload.message) return
      deliver(normalizeChatMessage(payload.message, Date.now()))
    })

    task.onError((event) => {
      if (closed) return
      if (isUnauthorized(event)) {
        stopUnauthorized()
        return
      }
      socket = null
      startPolling()
      scheduleReconnect()
    })

    task.onClose((event) => {
      if (closed) return
      if (isUnauthorized(event)) {
        stopUnauthorized()
        return
      }
      socket = null
      startPolling()
      scheduleReconnect()
    })
  }

  open()

  return {
    get closed() {
      return closed
    },
    close() {
      closed = true
      clearTimers()
      if (socket) {
        try { socket.close({}) } catch (e) {}
        socket = null
      }
    },
  }
}

module.exports = {
  fetchChatMessages,
  sendChatMessage,
  connectChatRoom,
  mergeChatMessages,
}
