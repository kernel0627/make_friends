const { TENCENT_MAP_KEY } = require('./config')

function compactText(value) {
  return String(value || '').replace(/\s+/g, '').trim()
}

function sameText(left, right) {
  const a = compactText(left)
  const b = compactText(right)
  return !!a && !!b && (a === b || a.indexOf(b) !== -1 || b.indexOf(a) !== -1)
}

function bestPoiTitle(result) {
  const pois = Array.isArray(result && result.pois) ? result.pois : []
  if (!pois.length) return ''

  const sorted = pois.slice().sort((left, right) => {
    const leftDistance = Number(left && left._distance)
    const rightDistance = Number(right && right._distance)
    return (Number.isFinite(leftDistance) ? leftDistance : 999999) - (Number.isFinite(rightDistance) ? rightDistance : 999999)
  })
  return compactText(sorted[0] && sorted[0].title)
}

function buildLocationText(result) {
  const formatted = (result && result.formatted_addresses) || {}
  const recommend = compactText(formatted.recommend)
  const rough = compactText(formatted.rough)
  const poiTitle = bestPoiTitle(result)
  const address = compactText(result && result.address)

  const name = recommend || poiTitle || rough || address
  if (!name) return ''
  if (!address || sameText(name, address)) return name
  return name + ' ' + address
}

function buildSuggestionAddress(item, parent) {
  const source = item || {}
  const fallback = parent || {}
  const adInfo = source.ad_info || fallback.ad_info || {}
  const parts = [
    source.address || fallback.address,
    adInfo.province || source.province || fallback.province,
    adInfo.city || source.city || fallback.city,
    adInfo.district || source.district || fallback.district,
  ].map(compactText).filter(Boolean)

  const uniqueParts = []
  parts.forEach((part) => {
    if (!uniqueParts.some((existing) => sameText(existing, part))) {
      uniqueParts.push(part)
    }
  })
  return uniqueParts.join(' ')
}

function suggestionTitle(item, parent) {
  const title = compactText(item && item.title)
  const parentTitle = compactText(parent && parent.title)
  if (!parentTitle || !title || sameText(title, parentTitle)) return title
  return parentTitle + '-' + title
}

function normalizeSuggestion(item, index, parent) {
  const source = item || {}
  const fallback = parent || {}
  const adInfo = source.ad_info || fallback.ad_info || {}
  const title = suggestionTitle(source, fallback)
  const displayAddress = buildSuggestionAddress(source, fallback)
  const location = source.location || fallback.location || {}
  const latitude = Number(location.lat)
  const longitude = Number(location.lng)

  return {
    id: compactText(source.id || '') || (title + '-' + index),
    title,
    address: compactText(source.address || fallback.address),
    displayAddress,
    province: compactText(adInfo.province || source.province || fallback.province),
    city: compactText(adInfo.city || source.city || fallback.city),
    district: compactText(adInfo.district || source.district || fallback.district),
    latitude: Number.isFinite(latitude) ? latitude : null,
    longitude: Number.isFinite(longitude) ? longitude : null,
    raw: item,
  }
}

function flattenSuggestions(list, limit) {
  const result = []
  list.forEach((item, index) => {
    result.push(normalizeSuggestion(item, index))
    const subPois = Array.isArray(item && item.sub_pois) ? item.sub_pois : []
    subPois.forEach((subItem, subIndex) => {
      result.push(normalizeSuggestion(subItem, index + '-' + subIndex, item))
    })
  })

  const seen = {}
  return result.filter((item) => {
    if (!item.title) return false
    const key = item.title + '|' + item.address
    if (seen[key]) return false
    seen[key] = true
    return true
  }).slice(0, limit)
}

function searchPlaceSuggestions(keyword, options) {
  const key = String(TENCENT_MAP_KEY || '').trim()
  if (!key) {
    return Promise.reject({ code: 'MAP_KEY_MISSING', errMsg: 'missing Tencent Map key' })
  }

  const text = String(keyword || '').trim()
  if (!text) return Promise.resolve([])

  const opts = options || {}
  return new Promise((resolve, reject) => {
    wx.request({
      url: 'https://apis.map.qq.com/ws/place/v1/suggestion',
      method: 'GET',
      data: {
        key,
        keyword: text,
        region: opts.region || '全国',
        region_fix: 0,
        get_subpois: 1,
        policy: 1,
        page_size: opts.pageSize || 8,
        page_index: 1,
      },
      success(res) {
        const data = (res && res.data) || {}
        if (data.status !== 0) {
          reject({
            code: 'PLACE_SUGGESTION_FAIL',
            status: data.status,
            errMsg: data.message || 'place suggestion failed',
          })
          return
        }

        const list = Array.isArray(data.data) ? data.data : []
        resolve(flattenSuggestions(list, opts.pageSize || 8))
      },
      fail(err) {
        reject({
          code: 'PLACE_SUGGESTION_NETWORK_FAIL',
          errMsg: (err && err.errMsg) || 'place suggestion network failed',
        })
      },
    })
  })
}

function reverseGeocode(latitude, longitude) {
  const key = String(TENCENT_MAP_KEY || '').trim()
  if (!key) {
    return Promise.reject({ code: 'MAP_KEY_MISSING', errMsg: 'missing Tencent Map key' })
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url: 'https://apis.map.qq.com/ws/geocoder/v1/',
      method: 'GET',
      data: {
        key,
        location: String(latitude) + ',' + String(longitude),
        get_poi: 1,
        poi_options: 'page_size=5;page_index=1',
      },
      success(res) {
        const data = (res && res.data) || {}
        if (data.status !== 0 || !data.result) {
          reject({
            code: 'REVERSE_GEOCODE_FAIL',
            status: data.status,
            errMsg: data.message || 'reverse geocode failed',
          })
          return
        }

        const text = buildLocationText(data.result)
        if (!text) {
          reject({ code: 'REVERSE_GEOCODE_EMPTY', errMsg: 'empty reverse geocode result' })
          return
        }
        resolve({
          address: text,
          raw: data.result,
        })
      },
      fail(err) {
        reject({
          code: 'REVERSE_GEOCODE_NETWORK_FAIL',
          errMsg: (err && err.errMsg) || 'reverse geocode network failed',
        })
      },
    })
  })
}

module.exports = {
  reverseGeocode,
  searchPlaceSuggestions,
}
