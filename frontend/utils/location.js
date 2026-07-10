const { reverseGeocode } = require('./mapApi')

const LOCATION_ADDRESS_CACHE_KEY = 'zgbe_location_address_cache'
const LAST_LOCATION_KEY = 'zgbe_last_location'

function formatCoordinateAddress(latitude, longitude) {
  const lat = Number(latitude)
  const lng = Number(longitude)
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return '当前位置'
  return '当前位置 ' + lat.toFixed(5) + ', ' + lng.toFixed(5)
}

function coordinateCacheKey(latitude, longitude) {
  const lat = Number(latitude)
  const lng = Number(longitude)
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return ''
  return lat.toFixed(4) + ',' + lng.toFixed(4)
}

function readAddressCache() {
  try {
    const value = wx.getStorageSync(LOCATION_ADDRESS_CACHE_KEY)
    return value && typeof value === 'object' ? value : {}
  } catch (e) {
    return {}
  }
}

function getCachedAddress(latitude, longitude) {
  const key = coordinateCacheKey(latitude, longitude)
  if (!key) return ''
  const cache = readAddressCache()
  return String((cache[key] && cache[key].address) || '').trim()
}

function saveCachedAddress(latitude, longitude, address) {
  const key = coordinateCacheKey(latitude, longitude)
  const text = String(address || '').trim()
  if (!key || !text) return
  try {
    const cache = readAddressCache()
    cache[key] = {
      address: text,
      updatedAt: Date.now(),
    }
    wx.setStorageSync(LOCATION_ADDRESS_CACHE_KEY, cache)
  } catch (e) {}
}

function normalizeLocation(value) {
  const source = value || {}
  const latitude = Number(source.latitude)
  const longitude = Number(source.longitude)
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) return null
  return {
    latitude,
    longitude,
    accuracy: Number(source.accuracy) || 0,
    address: String(source.address || formatCoordinateAddress(latitude, longitude)).trim(),
    updatedAt: Number(source.updatedAt) || Date.now(),
  }
}

function saveLastLocation(location) {
  const normalized = normalizeLocation(Object.assign({}, location || {}, { updatedAt: Date.now() }))
  if (!normalized) return
  try {
    wx.setStorageSync(LAST_LOCATION_KEY, normalized)
  } catch (e) {}
}

function getLastLocation(maxAgeMs) {
  try {
    const location = normalizeLocation(wx.getStorageSync(LAST_LOCATION_KEY))
    if (!location) return null
    const ageLimit = Number(maxAgeMs) || 24 * 60 * 60 * 1000
    if (Date.now() - location.updatedAt > ageLimit) return null
    return location
  } catch (e) {
    return null
  }
}

function clearLastLocation() {
  try {
    wx.removeStorageSync(LAST_LOCATION_KEY)
  } catch (e) {}
}

function getCurrentLocation() {
  return new Promise(function(resolve, reject) {
    wx.getLocation({
      type: 'gcj02',
      timeout: 10000,
      isHighAccuracy: true,
      success: function(res) {
        const location = {
          latitude: res.latitude,
          longitude: res.longitude,
          accuracy: res.accuracy || 0,
          address: formatCoordinateAddress(res.latitude, res.longitude),
          fallbackAddress: formatCoordinateAddress(res.latitude, res.longitude),
        }

        reverseGeocode(res.latitude, res.longitude)
          .then(function(geo) {
            const result = Object.assign({}, location, {
              address: geo.address || location.address,
              resolvedAddress: geo.address || '',
              rawAddress: geo.raw || null,
            })
            saveCachedAddress(res.latitude, res.longitude, geo.address)
            saveLastLocation(result)
            resolve(result)
          })
          .catch(function(err) {
            const cachedAddress = getCachedAddress(res.latitude, res.longitude)
            const result = Object.assign({}, location, {
              address: cachedAddress || location.address,
              cachedAddress: cachedAddress || '',
              reverseGeocodeError: err,
            })
            saveLastLocation(result)
            resolve(result)
          })
      },
      fail: function(err) {
        const errMsg = (err && err.errMsg) || ''
        if (errMsg.indexOf('timeout') !== -1) {
          reject({ code: 'LOCATION_TIMEOUT', errMsg })
          return
        }
        if (errMsg.indexOf('auth deny') !== -1 || errMsg.indexOf('auth denied') !== -1 || errMsg.indexOf('deny') !== -1) {
          reject({ code: 'LOCATION_DENIED', errMsg })
          return
        }
        reject({ code: 'LOCATION_FAIL', errMsg: errMsg || '定位失败' })
      },
    })
  })
}

module.exports = { getCurrentLocation, getLastLocation, clearLastLocation }
