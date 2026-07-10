const { openPage } = require('../../utils/navigation')
const { listPosts, reportFeedExposures, reportFeedClick } = require('../../utils/postApi')
const { getCurrentLocation, getLastLocation, clearLastLocation } = require('../../utils/location')
const { searchPlaceSuggestions } = require('../../utils/mapApi')

const SESSION_KEY = 'zgbe_feed_session'
const CATEGORY_MAP = {
  全部: [],
  学习: ['自习', '编程', '英语', '读书'],
  运动: ['跑步', '骑行', '篮球', '羽毛球'],
  娱乐: ['电影', 'KTV', '桌游', '逛展'],
  其他: ['探店', '宠物', '志愿', '摄影'],
}

function getFeedSessionId() {
  try {
    let sessionId = wx.getStorageSync(SESSION_KEY)
    if (sessionId) return sessionId
    sessionId = 'session_' + Date.now() + '_' + Math.random().toString(16).slice(2, 10)
    wx.setStorageSync(SESSION_KEY, sessionId)
    return sessionId
  } catch (e) {
    return 'session_' + Date.now()
  }
}

function toRadians(value) {
  return value * Math.PI / 180
}

function distanceBetween(a, b) {
  if (!a || !b) return Number.MAX_SAFE_INTEGER
  const earthRadius = 6371
  const dLat = toRadians(b.latitude - a.latitude)
  const dLng = toRadians(b.longitude - a.longitude)
  const lat1 = toRadians(a.latitude)
  const lat2 = toRadians(b.latitude)
  const x = Math.sin(dLat / 2) * Math.sin(dLat / 2)
  const y = Math.sin(dLng / 2) * Math.sin(dLng / 2) * Math.cos(lat1) * Math.cos(lat2)
  const c = 2 * Math.atan2(Math.sqrt(x + y), Math.sqrt(1 - x - y))
  return earthRadius * c
}

function buildLocationButtonText(address) {
  const value = String(address || '').trim()
  if (!value) return '地点'
  return value.length > 6 ? (value.slice(0, 6) + '...') : value
}

Page({
  data: {
    categories: ['全部', '学习', '运动', '娱乐', '其他'],
    activeCategory: '全部',
    subCategories: [],
    activeSubCategory: '',
    sortBy: 'hot',
    keyword: '',
    addressFilter: '',
    posts: [],
    page: 1,
    pageSize: 10,
    hasMore: false,
    nextPage: 0,
    loading: false,
    loadingMore: false,
    errorText: '',
    userCoords: null,
    userLocationAddress: '',
    locating: false,
    showLocationSheet: false,
    showManualLocationDialog: false,
    manualLocationText: '',
    manualLocationSuggestions: [],
    manualLocationSearching: false,
    manualLocationSearchError: '',
    locationButtonText: '地点',
  },

  onLoad() {
    const lastLocation = getLastLocation()
    if (lastLocation) {
      this.setData({
        userCoords: { latitude: lastLocation.latitude, longitude: lastLocation.longitude },
        userLocationAddress: lastLocation.address || '',
        locationButtonText: buildLocationButtonText(lastLocation.address || '当前位置'),
      })
    }
    this.refreshList(true)
  },

  onPullDownRefresh() {
    this.refreshList(true).finally(() => wx.stopPullDownRefresh())
  },

  onReachBottom() {
    if (this.data.hasMore && !this.data.loading && !this.data.loadingMore) {
      this.refreshList(false)
    }
  },

  currentQuery(page) {
    const query = {
      sortBy: this.data.sortBy === 'latest' ? 'latest' : 'hot',
      category: this.data.activeCategory !== '全部' ? this.data.activeCategory : '',
      subCategory: this.data.activeSubCategory || '',
      keyword: (this.data.keyword || '').trim(),
      addressKeyword: (this.data.addressFilter || '').trim(),
      page,
      pageSize: this.data.pageSize,
    }
    const coords = this.data.userCoords
    if (coords && Number.isFinite(Number(coords.latitude)) && Number.isFinite(Number(coords.longitude)) && query.sortBy === 'hot') {
      query.userLat = Number(coords.latitude)
      query.userLng = Number(coords.longitude)
    }
    return query
  },

  refreshList(reset) {
    const page = reset ? 1 : (this.data.nextPage || (this.data.page + 1))
    this._requestId = (this._requestId || 0) + 1
    const requestId = this._requestId

    this.setData({
      loading: reset,
      loadingMore: !reset,
      errorText: reset ? '' : this.data.errorText,
    })

    return listPosts(this.currentQuery(page))
      .then((res) => {
        if (requestId !== this._requestId) return

        let posts = Array.isArray(res.posts) ? res.posts.slice() : []
        if (this.data.sortBy === 'nearby' && this.data.userCoords) {
          const coords = this.data.userCoords
          posts = posts.slice().sort((left, right) => distanceBetween(coords, left.coords) - distanceBetween(coords, right.coords))
        }

        const baseOffset = reset ? 0 : this.data.posts.length
        const feedRequestId = res.feedRequestId || ''
        const decorated = posts.map((post, index) => Object.assign({}, post, {
          _feedRequestId: feedRequestId,
          _position: baseOffset + index + 1,
        }))
        const nextPosts = reset ? decorated : this.data.posts.concat(decorated)

        this.setData({
          posts: nextPosts,
          page: res.page || page,
          pageSize: res.pageSize || this.data.pageSize,
          hasMore: !!res.hasMore,
          nextPage: res.nextPage || 0,
          loading: false,
          loadingMore: false,
          errorText: '',
        })

        if (decorated.length && feedRequestId) {
          reportFeedExposures({
            feedRequestId,
            sessionId: getFeedSessionId(),
            items: decorated.map((item) => ({
              postId: item.id,
              position: item._position,
              strategy: this.data.sortBy === 'latest' ? 'latest' : 'personalized',
              score: item.recommendation && item.recommendation.score ? item.recommendation.score : 0,
            })),
          }).catch(() => null)
        }
      })
      .catch((err) => {
        if (requestId !== this._requestId) return
        this.setData({
          loading: false,
          loadingMore: false,
          posts: reset ? [] : this.data.posts,
          errorText: (err && err.message) || '加载活动失败，请稍后重试',
        })
      })
  },

  onCategoryChange(e) {
    const category = e.detail.category || '全部'
    this.setData({
      activeCategory: category,
      subCategories: CATEGORY_MAP[category] || [],
      activeSubCategory: '',
    })
    this.refreshList(true)
  },

  onSubCategoryChange(e) {
    this.setData({ activeSubCategory: e.detail.subCategory || '' })
    this.refreshList(true)
  },

  onSortChange(e) {
    const sortBy = e.detail.sortBy || 'hot'
    this.setData({ sortBy })
    this.refreshList(true)
  },

  onKeywordInput(e) {
    this.setData({ keyword: (e.detail && e.detail.value) || '' })
  },

  onKeywordConfirm() {
    this.refreshList(true)
  },

  onLocationFilterTap() {
    this.setData({ showLocationSheet: true })
  },

  onLocationSheetClose() {
    this.setData({ showLocationSheet: false })
  },

  onUseCurrentLocation() {
    if (this.data.locating) return
    this.setData({ showLocationSheet: false, locating: true })
    getCurrentLocation().then((res) => {
      const reverseCode = res.reverseGeocodeError && res.reverseGeocodeError.code
      const hasDetailedAddress = !!(res.resolvedAddress || res.cachedAddress)
      this.setData({
        userCoords: { latitude: res.latitude, longitude: res.longitude },
        userLocationAddress: res.address || '当前位置',
        addressFilter: '',
        manualLocationText: '',
        manualLocationSuggestions: [],
        manualLocationSearchError: '',
        manualLocationSearching: false,
        locationButtonText: buildLocationButtonText(res.address || '当前位置'),
      })
      wx.showToast({
        title: reverseCode && !hasDetailedAddress
          ? ((res.reverseGeocodeError && res.reverseGeocodeError.errMsg) || '已定位，将按距离推荐')
          : '已按当前位置优化推荐',
        icon: reverseCode && !hasDetailedAddress ? 'none' : 'success',
      })
      this.refreshList(true)
    }).catch((err) => {
      const code = err && err.code
      const title = code === 'LOCATION_DENIED'
        ? '请授权位置信息后重试'
        : code === 'LOCATION_TIMEOUT'
          ? '定位超时，请重试'
          : '获取定位失败，请手动输入地点'
      wx.showToast({ title, icon: 'none' })
    }).finally(() => {
      this.setData({ locating: false })
    })
  },

  onManualLocationTap() {
    this._clearManualLocationSearch()
    this.setData({
      showLocationSheet: false,
      showManualLocationDialog: true,
      manualLocationText: this.data.addressFilter || this.data.userLocationAddress || '',
      manualLocationSuggestions: [],
      manualLocationSearching: false,
      manualLocationSearchError: '',
    })
  },

  _clearManualLocationSearch() {
    if (this._manualLocationSearchTimer) {
      clearTimeout(this._manualLocationSearchTimer)
      this._manualLocationSearchTimer = null
    }
    this._manualLocationSearchSeq = (this._manualLocationSearchSeq || 0) + 1
  },

  onManualLocationInput(e) {
    const value = (e.detail && e.detail.value) || ''
    const keyword = value.trim()
    this._clearManualLocationSearch()
    this.setData({
      manualLocationText: value,
      manualLocationSuggestions: [],
      manualLocationSearchError: '',
    })

    if (keyword.length < 2) {
      this.setData({ manualLocationSearching: false })
      return
    }

    const seq = this._manualLocationSearchSeq
    this.setData({ manualLocationSearching: true })
    this._manualLocationSearchTimer = setTimeout(() => {
      this._searchManualLocationSuggestions(keyword, seq)
    }, 320)
  },

  _searchManualLocationSuggestions(keyword, seq) {
    searchPlaceSuggestions(keyword, { pageSize: 8 }).then((list) => {
      if (seq !== this._manualLocationSearchSeq) return
      this.setData({
        manualLocationSuggestions: list,
        manualLocationSearching: false,
        manualLocationSearchError: list.length ? '' : '没有找到匹配地点，可以继续手动输入',
      })
    }).catch((err) => {
      if (seq !== this._manualLocationSearchSeq) return
      this.setData({
        manualLocationSuggestions: [],
        manualLocationSearching: false,
        manualLocationSearchError: (err && err.errMsg) || '地点搜索失败，请稍后重试',
      })
    })
  },

  onManualLocationSuggestionTap(e) {
    const index = Number(e.currentTarget.dataset.index)
    const item = this.data.manualLocationSuggestions[index]
    if (!item) return

    const selectedText = item.title || item.displayAddress || item.address || ''
    const coords = item.latitude !== null && item.longitude !== null
      ? { latitude: item.latitude, longitude: item.longitude }
      : null

    this._clearManualLocationSearch()
    clearLastLocation()
    this.setData({
      addressFilter: coords ? '' : selectedText,
      manualLocationText: selectedText,
      manualLocationSuggestions: [],
      manualLocationSearchError: '',
      manualLocationSearching: false,
      userCoords: coords,
      userLocationAddress: item.displayAddress || item.address || selectedText,
      locationButtonText: buildLocationButtonText(selectedText),
      showManualLocationDialog: false,
    })
    this.refreshList(true)
  },

  onManualLocationCancel() {
    this._clearManualLocationSearch()
    this.setData({
      showManualLocationDialog: false,
      manualLocationSuggestions: [],
      manualLocationSearchError: '',
      manualLocationSearching: false,
    })
  },

  onManualLocationConfirm() {
    const addressFilter = (this.data.manualLocationText || '').trim()
    this._clearManualLocationSearch()
    clearLastLocation()
    this.setData({
      addressFilter,
      userCoords: null,
      userLocationAddress: '',
      manualLocationSuggestions: [],
      manualLocationSearchError: '',
      manualLocationSearching: false,
      locationButtonText: buildLocationButtonText(addressFilter),
      showManualLocationDialog: false,
    })
    this.refreshList(true)
  },

  onClearLocationFilter() {
    clearLastLocation()
    this.setData({
      addressFilter: '',
      manualLocationText: '',
      manualLocationSuggestions: [],
      manualLocationSearchError: '',
      manualLocationSearching: false,
      userCoords: null,
      userLocationAddress: '',
      locationButtonText: '地点',
      showLocationSheet: false,
      showManualLocationDialog: false,
    })
    this.refreshList(true)
  },

  onPostCardTap(e) {
    const detail = e.detail || {}
    const post = detail.post || (this.data.posts || []).find((item) => item.id === detail.postId)
    if (!post || !post.id) return

    if (post._feedRequestId) {
      reportFeedClick({
        feedRequestId: post._feedRequestId,
        sessionId: getFeedSessionId(),
        postId: post.id,
        position: post._position || 1,
        strategy: this.data.sortBy === 'latest' ? 'latest' : 'personalized',
        score: post.recommendation && post.recommendation.score ? post.recommendation.score : 0,
      }).catch(() => null)
    }

    openPage('/pages/detail/index?id=' + post.id)
  },

  onCalendarTap() {
    openPage('/pages/calendar/index')
  },

  onCreatePost() {
    openPage('/pages/post/index')
  },

  onChatTap() {
    wx.switchTab({ url: '/pages/chat/index' })
  },
})
