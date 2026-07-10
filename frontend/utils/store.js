const STORAGE_KEYS = {
  POSTS: 'zgbe_posts',
  CURRENT_USER: 'zgbe_current_user',
  JOINED_POST_IDS: 'zgbe_joined_post_ids',
  REVIEWS: 'zgbe_reviews',
  CHAT_MESSAGES: 'zgbe_chat_messages',
}

function normalizePosts(posts) {
  if (!Array.isArray(posts)) return []
  return posts.map(function(post) {
    var joinedUsers = Array.isArray(post.joinedUsers) ? post.joinedUsers : []
    var currentCount = typeof post.currentCount === 'number' ? post.currentCount : 0
    if (!joinedUsers.length && currentCount > 0 && post.author && post.author.id) {
      joinedUsers = [post.author]
    }
    return Object.assign({}, post, { joinedUsers: joinedUsers })
  })
}

function normalizeReviews(reviews) {
  if (!reviews || typeof reviews !== 'object') return {}
  var normalized = {}
  Object.keys(reviews).forEach(function(postId) {
    normalized[postId] = Array.isArray(reviews[postId]) ? reviews[postId] : []
  })
  return normalized
}

function normalizeChatMessages(chatMessages) {
  if (!chatMessages || typeof chatMessages !== 'object') return {}
  var normalized = {}
  Object.keys(chatMessages).forEach(function(postId) {
    normalized[postId] = Array.isArray(chatMessages[postId]) ? chatMessages[postId] : []
  })
  return normalized
}

function mergeChatMessages(baseMap, localMap) {
  var merged = {}
  var base = normalizeChatMessages(baseMap)
  var local = normalizeChatMessages(localMap)

  Object.keys(base).forEach(function(postId) {
    merged[postId] = base[postId].slice()
  })
  Object.keys(local).forEach(function(postId) {
    merged[postId] = local[postId].slice()
  })
  return merged
}

function safeGetStorage(key, fallbackValue) {
  try {
    var value = wx.getStorageSync(key)
    return value === '' || value === undefined ? fallbackValue : value
  } catch (e) {
    return fallbackValue
  }
}

function saveStorage(key, value) {
  try {
    wx.setStorageSync(key, value)
  } catch (e) {}
}

function loadAppData(initialPosts, mockUser, initialChatMessages) {
  var postsInStorage = safeGetStorage(STORAGE_KEYS.POSTS, null)
  var currentUserInStorage = safeGetStorage(STORAGE_KEYS.CURRENT_USER, null)
  var joinedPostIdsInStorage = safeGetStorage(STORAGE_KEYS.JOINED_POST_IDS, null)
  var reviewsInStorage = safeGetStorage(STORAGE_KEYS.REVIEWS, null)
  var chatMessagesInStorage = safeGetStorage(STORAGE_KEYS.CHAT_MESSAGES, null)

  var posts = normalizePosts(postsInStorage || initialPosts || [])
  var currentUser = currentUserInStorage || mockUser || null
  var joinedPostIds = Array.isArray(joinedPostIdsInStorage) ? joinedPostIdsInStorage : []
  var reviews = normalizeReviews(reviewsInStorage)
  var chatMessages = mergeChatMessages(initialChatMessages || {}, chatMessagesInStorage)

  return {
    posts: posts,
    currentUser: currentUser,
    joinedPostIds: joinedPostIds,
    reviews: reviews,
    chatMessages: chatMessages,
  }
}

function savePosts(posts) {
  saveStorage(STORAGE_KEYS.POSTS, normalizePosts(posts))
}

function saveCurrentUser(currentUser) {
  saveStorage(STORAGE_KEYS.CURRENT_USER, currentUser || null)
}

function saveJoinedPostIds(joinedPostIds) {
  saveStorage(STORAGE_KEYS.JOINED_POST_IDS, Array.isArray(joinedPostIds) ? joinedPostIds : [])
}

function saveReviews(reviews) {
  saveStorage(STORAGE_KEYS.REVIEWS, normalizeReviews(reviews))
}

function saveChatMessages(chatMessages) {
  saveStorage(STORAGE_KEYS.CHAT_MESSAGES, normalizeChatMessages(chatMessages))
}

module.exports = {
  loadAppData: loadAppData,
  savePosts: savePosts,
  saveCurrentUser: saveCurrentUser,
  saveJoinedPostIds: saveJoinedPostIds,
  saveReviews: saveReviews,
  saveChatMessages: saveChatMessages,
}
