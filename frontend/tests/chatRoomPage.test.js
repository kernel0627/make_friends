const test = require('node:test')
const assert = require('node:assert/strict')

test('onLoad and first onShow share one chat initialization', async (t) => {
  const chatApiPath = require.resolve('../utils/chatApi')
  const authPath = require.resolve('../utils/auth')
  const navigationPath = require.resolve('../utils/navigation')
  const storePath = require.resolve('../utils/store')
  const pagePath = require.resolve('../pages/chat-room/index')
  const previous = new Map()
  for (const path of [chatApiPath, authPath, navigationPath, storePath, pagePath]) {
    previous.set(path, require.cache[path])
  }

  const originalPage = global.Page
  const originalWx = global.wx
  const originalGetApp = global.getApp
  t.after(() => {
    global.Page = originalPage
    global.wx = originalWx
    global.getApp = originalGetApp
    for (const [path, value] of previous) {
      if (value) require.cache[path] = value
      else delete require.cache[path]
    }
  })

  let fetchCount = 0
  let connectCount = 0
  require.cache[chatApiPath] = {
    exports: {
      fetchChatMessages() {
        fetchCount += 1
        return Promise.resolve([{ id: 'm1', createdAt: 1000 }])
      },
      sendChatMessage() {
        return Promise.resolve(null)
      },
      connectChatRoom() {
        connectCount += 1
        return { closed: false, close() { this.closed = true } }
      },
      mergeChatMessages(existing, incoming) {
        const items = new Map()
        ;(existing || []).concat(incoming || []).forEach((item) => items.set(item.id, item))
        return Array.from(items.values()).sort((a, b) => a.createdAt - b.createdAt)
      },
    },
  }
  require.cache[authPath] = {
    exports: {
      ensurePageLogin() { return true },
      loginWithWechat() { return Promise.resolve() },
      gotoLoginPage() {},
      gotoRegisterPage() {},
    },
  }
  require.cache[navigationPath] = { exports: { openPage() {} } }
  require.cache[storePath] = { exports: { saveChatMessages() {} } }

  let definition = null
  global.Page = (value) => {
    definition = value
  }
  global.wx = {
    setNavigationBarTitle() {},
    showToast() {},
  }
  global.getApp = () => ({ globalData: { chatMessages: {} } })
  delete require.cache[pagePath]
  require(pagePath)

  const page = {
    ...definition,
    data: { ...definition.data },
    setData(patch) {
      this.data = { ...this.data, ...patch }
    },
  }
  page.onLoad({ id: 'post-1', title: 'room' })
  const firstInitialization = page._initializing
  page.onShow()
  assert.equal(page._initializing, firstInitialization)
  await firstInitialization

  assert.equal(fetchCount, 1)
  assert.equal(connectCount, 1)
})
