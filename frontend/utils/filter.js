function haversineDistance(lat1, lon1, lat2, lon2) {
  const R = 6371
  const dLat = (lat2 - lat1) * Math.PI / 180
  const dLon = (lon2 - lon1) * Math.PI / 180
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
    Math.sin(dLon / 2) * Math.sin(dLon / 2)
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
  return R * c
}

function filterPosts(posts, options) {
  const opts = options || {}
  return posts.filter(function(post) {
    if (opts.keyword && !(post.title || '').includes(opts.keyword)) {
      return false
    }
    if (opts.category && opts.category !== '热门' && post.category !== opts.category) {
      return false
    }
    if (opts.subCategory && post.subCategory !== opts.subCategory) {
      return false
    }
    if (opts.address && !(post.address || '').includes(opts.address)) {
      return false
    }
    return true
  })
}

function sortPosts(posts, sortBy, userCoords) {
  let arr = posts.slice()
  if (sortBy === 'hot') {
    const now = Date.now()
    arr.sort(function(a, b) {
      const scoreA = hotScore(a, now)
      const scoreB = hotScore(b, now)
      if (scoreA === scoreB) {
        return (b.createdAt || 0) - (a.createdAt || 0)
      }
      return scoreB - scoreA
    })
  } else if (sortBy === 'latest') {
    arr.sort(function(a, b) { return (b.createdAt || 0) - (a.createdAt || 0) })
  } else if (sortBy === 'nearby' && userCoords) {
    arr = arr.map(function(post) {
      const dist = post.coords
        ? haversineDistance(userCoords.latitude, userCoords.longitude, post.coords.latitude, post.coords.longitude)
        : Infinity
      return Object.assign({}, post, { distance: dist })
    })
    arr.sort(function(a, b) { return (a.distance || Infinity) - (b.distance || Infinity) })
  }
  return arr
}

function hotScore(post, nowTs) {
  const currentCount = Number(post && post.currentCount) || 0
  const createdAt = Number(post && post.createdAt) || nowTs
  const ageMs = Math.max(0, nowTs - createdAt)
  const recentWindowMs = 48 * 60 * 60 * 1000
  let recencyBoost = 0
  if (ageMs < recentWindowMs) {
    const ageHours = ageMs / (60 * 60 * 1000)
    recencyBoost = ((48 - ageHours) / 48) * 5
    if (recencyBoost < 0) recencyBoost = 0
  }
  return currentCount * 2 + recencyBoost
}

module.exports = { filterPosts, sortPosts }
