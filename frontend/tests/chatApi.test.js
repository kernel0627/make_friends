const test = require('node:test')
const assert = require('node:assert/strict')

const chatApi = require('../utils/chatApi')

test('mergeChatMessages deduplicates an inclusive cursor boundary by ID', () => {
  const existing = [
    { id: 'a', createdAt: 1000, content: 'first' },
    { id: 'b', createdAt: 2000, content: 'boundary' },
  ]
  const incoming = [
    { id: 'b', createdAt: 2000, content: 'boundary' },
    { id: 'c', createdAt: 2000, content: 'new at same millisecond' },
  ]

  assert.deepEqual(
    chatApi.mergeChatMessages(existing, incoming).map((item) => item.id),
    ['a', 'b', 'c'],
  )
})

test('fetchChatMessages sends an incremental cursor and caps its limit', async (t) => {
  const originalWx = global.wx
  t.after(() => {
    global.wx = originalWx
  })

  let requestOptions = null
  global.wx = {
    getStorageSync(key) {
      return key === 'zgbe_access_token' ? 'access-token' : ''
    },
    request(options) {
      requestOptions = options
      options.success({ statusCode: 200, data: { messages: [] } })
    },
  }

  await chatApi.fetchChatMessages('post/with space', {
    sinceCreatedAt: 1234,
    limit: 900,
  })

  assert.equal(requestOptions.url.endsWith('/chats/post%2Fwith%20space/messages'), true)
  assert.deepEqual(requestOptions.data, {
    sinceCreatedAt: 1234,
    limit: 500,
  })
})

test('a polling 401 closes the chat state machine and cancels reconnects', async (t) => {
  const originalWx = global.wx
  const originalGetApp = global.getApp
  const originalSetInterval = global.setInterval
  const originalClearInterval = global.clearInterval
  const originalSetTimeout = global.setTimeout
  const originalClearTimeout = global.clearTimeout
  t.after(() => {
    global.wx = originalWx
    global.getApp = originalGetApp
    global.setInterval = originalSetInterval
    global.clearInterval = originalClearInterval
    global.setTimeout = originalSetTimeout
    global.clearTimeout = originalClearTimeout
  })

  let poll = null
  let intervalCleared = false
  let reconnectCleared = false
  let socketHandlers = {}
  const statuses = []

  global.setInterval = (callback) => {
    poll = callback
    return 11
  }
  global.clearInterval = () => {
    intervalCleared = true
  }
  global.setTimeout = () => 22
  global.clearTimeout = () => {
    reconnectCleared = true
  }
  global.getApp = () => ({ globalData: {} })
  global.wx = {
    getStorageSync(key) {
      return key === 'zgbe_access_token' ? 'access-token' : ''
    },
    setStorageSync() {},
    showToast() {},
    switchTab(options) {
      if (options && options.complete) options.complete()
    },
    request(options) {
      options.success({
        statusCode: 401,
        data: { code: 'ACCESS_TOKEN_REVOKED', message: 'revoked' },
      })
    },
    connectSocket() {
      return {
        onOpen(callback) { socketHandlers.open = callback },
        onMessage(callback) { socketHandlers.message = callback },
        onError(callback) { socketHandlers.error = callback },
        onClose(callback) { socketHandlers.close = callback },
        close() {},
      }
    },
  }

  const connection = chatApi.connectChatRoom('post-1', {
    sinceCreatedAt: 1000,
    onStatusChange(status) {
      statuses.push(status)
    },
  })
  socketHandlers.error({ errMsg: 'network unavailable' })
  assert.equal(typeof poll, 'function')

  poll()
  await new Promise((resolve) => setImmediate(resolve))

  assert.equal(connection.closed, true)
  assert.equal(intervalCleared, true)
  assert.equal(reconnectCleared, true)
  assert.equal(statuses.includes('unauthorized'), true)
})
