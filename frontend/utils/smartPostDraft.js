const ACTIVITY_RULES = [
  { category: '运动', subCategory: '羽毛球', keywords: ['羽毛球', '羽球'] },
  { category: '运动', subCategory: '篮球', keywords: ['篮球'] },
  { category: '运动', subCategory: '足球', keywords: ['足球'] },
  { category: '运动', subCategory: '跑步', keywords: ['跑步', '夜跑', '晨跑'] },
  { category: '运动', subCategory: '骑行', keywords: ['骑行', '单车'] },
  { category: '运动', subCategory: '游泳', keywords: ['游泳'] },
  { category: '娱乐', subCategory: '桌游', keywords: ['桌游', '剧本杀', '狼人杀', '棋牌'] },
  { category: '娱乐', subCategory: '电影', keywords: ['电影', '看电影', '观影'] },
  { category: '娱乐', subCategory: 'KTV', keywords: ['KTV', '唱歌'] },
  { category: '娱乐', subCategory: '游戏', keywords: ['游戏', '开黑'] },
  { category: '学习', subCategory: '英语', keywords: ['英语', '口语'] },
  { category: '学习', subCategory: '考研', keywords: ['考研'] },
  { category: '学习', subCategory: '编程', keywords: ['编程', '代码', '刷题'] },
  { category: '学习', subCategory: '读书', keywords: ['读书', '自习', '学习'] },
]

const CITY_NAMES = ['北京', '上海', '广州', '深圳', '杭州', '成都', '武汉', '南京', '苏州', '西安', '重庆', '天津']
const CN_NUMBERS = {
  一: 1,
  二: 2,
  两: 2,
  三: 3,
  四: 4,
  五: 5,
  六: 6,
  七: 7,
  八: 8,
  九: 9,
  十: 10,
}
const PAST_DATE_POLICY = {
  NEXT_YEAR: 'nextYear',
  BLOCK: 'block',
}
const WEEKDAY_MAP = {
  日: 0,
  天: 0,
  一: 1,
  二: 2,
  三: 3,
  四: 4,
  五: 5,
  六: 6,
}

function compact(value) {
  return String(value || '').replace(/\s+/g, '').trim()
}

function trimText(value) {
  return String(value || '').trim()
}

function parseCNNumber(value) {
  const text = compact(value)
  if (!text) return 0
  const numeric = parseInt(text, 10)
  if (Number.isFinite(numeric)) return numeric
  if (text === '十') return 10
  if (text.indexOf('十') !== -1) {
    const parts = text.split('十')
    const tens = parts[0] ? (CN_NUMBERS[parts[0]] || 0) : 1
    const ones = parts[1] ? (CN_NUMBERS[parts[1]] || 0) : 0
    return tens * 10 + ones
  }
  return CN_NUMBERS[text] || 0
}

function pad2(value) {
  return ('0' + String(value)).slice(-2)
}

function formatDate(date) {
  return date.getFullYear() + '-' + pad2(date.getMonth() + 1) + '-' + pad2(date.getDate())
}

function formatClock(hour, minute) {
  return pad2(hour) + ':' + pad2(minute || 0)
}

function formatFixedDisplay(date) {
  return formatDate(date) + ' ' + formatClock(date.getHours(), date.getMinutes())
}

function addDays(base, days) {
  const date = new Date(base.getTime())
  date.setDate(date.getDate() + days)
  return date
}

function startOfDayWithClock(base, hour, minute) {
  const date = new Date(base.getFullYear(), base.getMonth(), base.getDate(), hour, minute || 0, 0, 0)
  return date
}

function detectActivity(text) {
  const source = compact(text).toLowerCase()
  for (let i = 0; i < ACTIVITY_RULES.length; i++) {
    const rule = ACTIVITY_RULES[i]
    for (let j = 0; j < rule.keywords.length; j++) {
      if (source.indexOf(rule.keywords[j].toLowerCase()) !== -1) {
        return Object.assign({}, rule)
      }
    }
  }
  return null
}

function detectRangeDays(text) {
  const source = compact(text)
  const dayMatch = source.match(/([0-9一二两三四五六七八九十]+)天(内|以内|之内)?/)
  if (dayMatch) {
    const days = parseCNNumber(dayMatch[1])
    if (days > 0) return { days: Math.min(days, 30), source: dayMatch[0] }
  }
  const weekMatch = source.match(/([0-9一二两三四五六七八九十]*)周(内|以内|之内)/)
  if (weekMatch) {
    const weeks = parseCNNumber(weekMatch[1] || '一') || 1
    return { days: Math.min(weeks * 7, 30), source: weekMatch[0] }
  }
  if (/这两天|近两天/.test(source)) return { days: 2, source: '这两天' }
  if (/这几天|最近几天|这阵子/.test(source)) return { days: 3, source: '这几天' }
  if (/最近|近期|有空/.test(source)) return { days: 7, source: '最近' }
  if (/周末|这周末|本周末/.test(source)) return { days: 7, source: '周末' }
  if (/下周|下星期|下礼拜/.test(source)) return { days: 14, source: '下周' }
  return null
}

function detectDateHint(text, now, options) {
  const source = compact(text)
  const policy = (options && options.pastDatePolicy) || PAST_DATE_POLICY.BLOCK
  const fullDateMatch = source.match(/(20\d{2})[年\-/.](1[0-2]|0?[1-9])[月\-/.](3[01]|[12]\d|0?[1-9])日?号?/)
  if (fullDateMatch) {
    const year = parseInt(fullDateMatch[1], 10)
    const month = parseInt(fullDateMatch[2], 10)
    const day = parseInt(fullDateMatch[3], 10)
    const target = new Date(year, month - 1, day)
    if (target.getFullYear() === year && target.getMonth() === month - 1 && target.getDate() === day) {
      if (target.getTime() < new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()) {
        return { exact: true, invalidPastDate: true, source: fullDateMatch[0], displayDate: year + '年' + month + '月' + day + '日' }
      }
      return { exact: true, targetDate: target, source: fullDateMatch[0] }
    }
  }

  const monthDayMatch = source.match(/(1[0-2]|0?[1-9])月(3[01]|[12]\d|0?[1-9])(?:日|号)?/)
  if (monthDayMatch) {
    const month = parseInt(monthDayMatch[1], 10)
    const day = parseInt(monthDayMatch[2], 10)
    let year = now.getFullYear()
    let target = new Date(year, month - 1, day)
    if (target.getFullYear() !== year || target.getMonth() !== month - 1 || target.getDate() !== day) {
      return null
    }
    if (target.getTime() < new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()) {
      if (policy !== PAST_DATE_POLICY.NEXT_YEAR) {
        return { exact: true, invalidPastDate: true, source: monthDayMatch[0], displayDate: month + '月' + day + '日' }
      }
      year += 1
      target = new Date(year, month - 1, day)
    }
    return { exact: true, targetDate: target, source: monthDayMatch[0] }
  }

  const slashDateMatch = source.match(/(1[0-2]|0?[1-9])[\/.](3[01]|[12]\d|0?[1-9])/)
  if (slashDateMatch) {
    const month = parseInt(slashDateMatch[1], 10)
    const day = parseInt(slashDateMatch[2], 10)
    let year = now.getFullYear()
    let target = new Date(year, month - 1, day)
    if (target.getFullYear() !== year || target.getMonth() !== month - 1 || target.getDate() !== day) {
      return null
    }
    if (target.getTime() < new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()) {
      if (policy !== PAST_DATE_POLICY.NEXT_YEAR) {
        return { exact: true, invalidPastDate: true, source: slashDateMatch[0], displayDate: month + '/' + day }
      }
      year += 1
      target = new Date(year, month - 1, day)
    }
    return { exact: true, targetDate: target, source: slashDateMatch[0] }
  }

  if (/大后天/.test(source)) return { exact: true, dayOffset: 3, source: '大后天' }
  if (/后天/.test(source)) return { exact: true, dayOffset: 2, source: '后天' }
  if (/明天|明日|明晚|明早/.test(source)) return { exact: true, dayOffset: 1, source: '明天' }
  if (/今天|今晚|今早|今下午|今晚上/.test(source)) return { exact: true, dayOffset: 0, source: '今天' }

  const weekdayMatch = source.match(/(下)?(?:周|星期|礼拜)([一二三四五六日天])/)
  if (weekdayMatch) {
    const target = WEEKDAY_MAP[weekdayMatch[2]]
    let offset = (target - now.getDay() + 7) % 7
    if (weekdayMatch[1] || offset === 0) offset += 7
    return { exact: true, dayOffset: offset, weekday: target, source: weekdayMatch[0] }
  }
  return null
}

function detectClockHint(text) {
  const source = compact(text)
  const hasMorning = /凌晨|早上|早|上午|明早|今早|后天早/.test(source)
  const hasNoon = /中午/.test(source)
  const hasAfternoon = /下午/.test(source)
  const hasNight = /晚上|晚|夜|今晚|明晚|今晚上/.test(source)
  const clockMatch = source.match(/([01]?\d|2[0-3])(?:[:：点时])([0-5]\d)?/)
  if (clockMatch) {
    let hour = parseInt(clockMatch[1], 10)
    const minute = clockMatch[2] ? parseInt(clockMatch[2], 10) : 0
    if ((hasAfternoon || hasNight) && hour < 12) hour += 12
    if (hasNoon && hour < 11) hour += 12
    return { explicit: true, hour, minute, source: clockMatch[0] }
  }
  if (hasMorning) return { explicit: false, hour: 9, minute: 0, source: '早上' }
  if (hasNoon) return { explicit: false, hour: 12, minute: 0, source: '中午' }
  if (hasAfternoon) return { explicit: false, hour: 14, minute: 0, source: '下午' }
  if (hasNight) return { explicit: false, hour: 19, minute: 0, source: '晚上' }
  return null
}

function detectPeopleCount(text) {
  const source = compact(text)
  const match = source.match(/([0-9一二两三四五六七八九十]+)(?:个)?(?:人|位)/)
  if (!match) return 0
  const count = parseCNNumber(match[1])
  return count >= 2 ? Math.min(count, 99) : 0
}

function detectExplicitLocation(text, activity) {
  const source = trimText(text)
  const match = source.match(/(?:在|去|到)([^，,。；;]+)/)
  if (!match) return ''
  let location = trimText(match[1])
  ;(activity.keywords || []).forEach((keyword) => {
    location = location.replace(new RegExp(keyword, 'g'), '')
  })
  location = location
    .replace(/(打|玩|看|一起|组织|来|约|局|活动)/g, '')
    .replace(/(今天|今晚|明天|明晚|后天|大后天|周末|下周|最近|这几天)/g, '')
    .replace(/([0-9一二两三四五六七八九十]+)(点|时|:|：)([0-5][0-9])?/g, '')
    .replace(/([0-9一二两三四五六七八九十]+)(个)?(人|位)/g, '')
    .trim()
  return location.length >= 2 ? location : ''
}

function postMatchesActivity(post, activity) {
  if (!post || !activity) return false
  const subCategory = trimText(post.subCategory)
  const category = trimText(post.category)
  if (activity.subCategory) return category === activity.category && subCategory === activity.subCategory
  return category === activity.category && !subCategory
}

function postWeight(item, now) {
  const base = item.role === 'initiated' ? 2 : 1
  const createdAt = Number(item.post && item.post.createdAt) || now.getTime()
  const ageDays = Math.max(0, (now.getTime() - createdAt) / 86400000)
  return base * (1 + 1 / (1 + ageDays / 30))
}

function collectHistory(history, activity, now) {
  const rows = []
  ;(history.initiatedPosts || []).forEach((post) => rows.push({ post, role: 'initiated' }))
  ;(history.joinedPosts || []).forEach((post) => rows.push({ post, role: 'joined' }))
  return rows
    .filter((item) => postMatchesActivity(item.post, activity))
    .map((item) => Object.assign({}, item, { weight: postWeight(item, now) }))
}

function bucketBest(items, getKey, getPayload) {
  const buckets = {}
  items.forEach((item) => {
    const key = getKey(item)
    if (!key) return
    if (!buckets[key]) {
      buckets[key] = { key, score: 0, count: 0, payload: getPayload ? getPayload(item) : key }
    }
    buckets[key].score += item.weight || 1
    buckets[key].count += 1
  })
  return Object.keys(buckets)
    .map((key) => buckets[key])
    .sort((a, b) => {
      if (b.score === a.score) return b.count - a.count
      return b.score - a.score
    })[0] || null
}

function cityFromText(value) {
  const text = trimText(value)
  for (let i = 0; i < CITY_NAMES.length; i++) {
    if (text.indexOf(CITY_NAMES[i]) !== -1) return CITY_NAMES[i]
  }
  return ''
}

function distanceKm(a, b) {
  if (!a || !b) return Infinity
  const lat1 = Number(a.latitude)
  const lng1 = Number(a.longitude)
  const lat2 = Number(b.latitude)
  const lng2 = Number(b.longitude)
  if (![lat1, lng1, lat2, lng2].every(Number.isFinite)) return Infinity
  const toRad = (value) => value * Math.PI / 180
  const dLat = toRad(lat2 - lat1)
  const dLng = toRad(lng2 - lng1)
  const rLat1 = toRad(lat1)
  const rLat2 = toRad(lat2)
  const h = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(rLat1) * Math.cos(rLat2) * Math.sin(dLng / 2) * Math.sin(dLng / 2)
  return 6371 * 2 * Math.atan2(Math.sqrt(Math.min(1, h)), Math.sqrt(Math.max(0, 1 - h)))
}

function validCoords(coords) {
  if (!coords) return false
  const lat = Number(coords.latitude)
  const lng = Number(coords.longitude)
  return Number.isFinite(lat) && Number.isFinite(lng) && !(lat === 0 && lng === 0)
}

function selectHistoryLocation(items, currentLocation) {
  const currentCity = cityFromText(currentLocation && currentLocation.address)
  let filteredFarCount = 0
  const candidates = items.filter((item) => {
    const post = item.post || {}
    const address = trimText(post.address)
    if (!address) return false
    if (!currentLocation) return true
    if (validCoords(post.coords)) {
      const km = distanceKm(currentLocation, post.coords)
      item.distanceKm = km
      if (km <= 30) {
        item.weight *= km <= 10 ? 1.2 : 0.8
        return true
      }
      filteredFarCount += 1
      return false
    }
    const postCity = cityFromText(address)
    if (currentCity && postCity && currentCity === postCity) {
      item.weight *= 0.6
      return true
    }
    filteredFarCount += 1
    return false
  })

  const best = bucketBest(candidates, (item) => trimText(item.post.address), (item) => ({
    address: trimText(item.post.address),
    coords: validCoords(item.post.coords) ? item.post.coords : null,
    distanceKm: Number.isFinite(item.distanceKm) ? item.distanceKm : null,
  }))
  return {
    best: best && best.payload,
    filteredFarCount,
  }
}

function selectHistoryClock(items) {
  const fixedItems = items.filter((item) => item.post && item.post.timeInfo && item.post.timeInfo.fixedTime)
  const best = bucketBest(fixedItems, (item) => {
    const date = new Date(item.post.timeInfo.fixedTime)
    if (!Number.isFinite(date.getTime())) return ''
    return date.getDay() + '-' + date.getHours() + '-' + date.getMinutes()
  }, (item) => {
    const date = new Date(item.post.timeInfo.fixedTime)
    return { weekday: date.getDay(), hour: date.getHours(), minute: date.getMinutes() }
  })
  if (best) return best.payload

  const bestHour = bucketBest(fixedItems, (item) => {
    const date = new Date(item.post.timeInfo.fixedTime)
    if (!Number.isFinite(date.getTime())) return ''
    return String(date.getHours())
  }, (item) => {
    const date = new Date(item.post.timeInfo.fixedTime)
    return { weekday: null, hour: date.getHours(), minute: 0 }
  })
  return bestHour && bestHour.payload
}

function selectHistoryRange(items) {
  const best = bucketBest(items, (item) => {
    const info = item.post && item.post.timeInfo
    if (!info || info.mode !== 'range') return ''
    const days = Number(info.days) || 0
    return days > 0 ? String(days) : ''
  }, (item) => Number(item.post.timeInfo.days) || 7)
  return best ? best.payload : 0
}

function selectHistoryMaxCount(items) {
  const best = bucketBest(items, (item) => {
    const count = Number(item.post && item.post.maxCount) || 0
    return count >= 2 ? String(count) : ''
  }, (item) => Number(item.post.maxCount) || 2)
  return best ? best.payload : 0
}

function nextDateForWeekday(now, weekday, hour, minute) {
  let offset = (weekday - now.getDay() + 7) % 7
  let date = addDays(now, offset)
  date = startOfDayWithClock(date, hour, minute || 0)
  if (date.getTime() <= now.getTime()) {
    date = addDays(date, 7)
  }
  return date
}

function resolveTime(intent, historyClock, historyRange, now, notes, options) {
  const dateHint = detectDateHint(intent.text, now, options)
  const rangeHint = detectRangeDays(intent.text)
  const clockHint = detectClockHint(intent.text)

  if (dateHint && dateHint.invalidPastDate) {
    return {
      error: 'PAST_DATE_NEEDS_CONFIRM',
      message: '你输入的日期 ' + dateHint.displayDate + ' 已经过去。请确认是否要按明年同一天生成，或返回修改日期。',
      source: dateHint.source,
    }
  }

  if (dateHint && dateHint.exact) {
    const hour = clockHint ? clockHint.hour : (historyClock ? historyClock.hour : 19)
    const minute = clockHint ? clockHint.minute : (historyClock ? historyClock.minute : 0)
    const baseDate = dateHint.targetDate || addDays(now, dateHint.dayOffset)
    let date = startOfDayWithClock(baseDate, hour, minute)
    if (date.getTime() <= now.getTime()) {
      notes.push('识别到的固定时间已经过去，请确认时间是否需要调整。')
      return { mode: 'range', days: 1, source: '时间已过，临时转为 1 天内' }
    }
    if (!clockHint && historyClock) {
      notes.push('时间点参考了你历史同类活动的常用时间。')
    } else if (!clockHint) {
      notes.push('没有说具体几点，默认补为晚上 7 点。')
    }
    return { mode: 'fixed', date, source: dateHint.source }
  }

  if (rangeHint) {
    return { mode: 'range', days: rangeHint.days, source: rangeHint.source }
  }

  if (clockHint && historyClock && historyClock.weekday !== null && historyClock.weekday !== undefined) {
    const date = nextDateForWeekday(now, historyClock.weekday, clockHint.hour, clockHint.minute)
    notes.push('你说了时间点但没说日期，日期参考了历史同类活动的常见星期。')
    return { mode: 'fixed', date, source: '历史星期 + 输入时间点' }
  }

  if (historyClock && historyClock.weekday !== null && historyClock.weekday !== undefined) {
    const date = nextDateForWeekday(now, historyClock.weekday, historyClock.hour, historyClock.minute)
    notes.push('没有说日期时间，已参考历史同类活动的常见星期和时间。')
    return { mode: 'fixed', date, source: '历史常用时间' }
  }

  if (historyRange) {
    notes.push('没有明确时间，已参考历史同类活动的常用活动周期。')
    return { mode: 'range', days: historyRange, source: '历史常用周期' }
  }

  return { mode: 'range', days: 7, source: '默认 7 天内' }
}

function timeLabel(timeInfo) {
  if (!timeInfo) return '最近几天'
  if (timeInfo.mode === 'range') return '未来 ' + timeInfo.days + ' 天内'
  const date = timeInfo.date
  const hour = date.getHours()
  const period = hour < 11 ? '上午' : hour < 13 ? '中午' : hour < 18 ? '下午' : '晚上'
  const now = new Date()
  const dateText = date.getFullYear() === now.getFullYear()
    ? ((date.getMonth() + 1) + '月' + date.getDate() + '日')
    : (date.getFullYear() + '年' + (date.getMonth() + 1) + '月' + date.getDate() + '日')
  return dateText + ' ' + period + formatClock(hour, date.getMinutes())
}

function shortTitleTime(timeInfo) {
  if (!timeInfo || timeInfo.mode === 'range') return ''
  const date = timeInfo.date
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const day = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
  const offset = Math.round((day - today) / 86400000)
  const hour = date.getHours()
  const period = hour < 11 ? '早上' : hour < 13 ? '中午' : hour < 18 ? '下午' : '晚上'
  if (offset === 0) return '今晚'
  if (offset === 1) return '明天' + period
  if (offset === 2) return '后天' + period
  return (date.getMonth() + 1) + '月' + date.getDate() + '日' + period
}

function shortLocation(address) {
  const text = trimText(address)
  if (!text) return ''
  const compacted = text.replace(/(北京市|上海市|广州市|深圳市|杭州市|成都市|武汉市|南京市|苏州市|西安市|重庆市|天津市)/g, '')
  const mainName = compacted.replace(/[（(][^）)]*[）)]/g, '').trim() || compacted
  return mainName.length > 10 ? mainName.slice(0, 10) : mainName
}

function buildDescription(activity, timeInfo, address, maxCount) {
  const when = timeLabel(timeInfo)
  const where = address ? '在' + address : '地点待补充，'
  const countText = maxCount ? '人数上限 ' + maxCount + ' 人' : '人数可再确认'
  const sub = activity.subCategory
  if (sub === '羽毛球') {
    return when + ' ' + where + '打羽毛球，' + countText + '。欢迎有空的同学一起参加，建议提前到场热身，自带球拍更方便。'
  }
  if (sub === '篮球') {
    return when + ' ' + where + '打篮球，' + countText + '。轻松组局，注意提前热身，现场按人数灵活分队。'
  }
  if (sub === '桌游') {
    return when + ' ' + where + '玩桌游，' + countText + '。新手友好，具体游戏可以到场后一起商量。'
  }
  if (sub === '电影') {
    return when + ' ' + where + '看电影，' + countText + '。可以提前在群里确认场次和座位。'
  }
  if (activity.category === '学习') {
    return when + ' ' + where + '一起' + sub + '，' + countText + '。互相监督，保持安静高效，有问题可以一起讨论。'
  }
  return when + ' ' + where + '一起' + sub + '，' + countText + '。感兴趣的朋友可以报名参加，具体安排进群后再确认。'
}

function parsePostTimeInfo(fields) {
  const source = fields || {}
  const mode = source.timeMode || 'range'
  if (mode === 'fixed') {
    const date = new Date(source.fixedTime || '')
    if (!Number.isFinite(date.getTime())) return null
    return { mode: 'fixed', date }
  }
  const days = parseInt(source.timeRange, 10) || 7
  return { mode: 'range', days: Math.max(1, Math.min(days, 30)) }
}

function buildSmartPostTitle(fields) {
  const source = fields || {}
  const subCategory = trimText(source.subCategory)
  const timeInfo = parsePostTimeInfo(source)
  if (!subCategory || !timeInfo) return ''
  const titleParts = [shortTitleTime(timeInfo), shortLocation(source.locationText), subCategory + '局']
  return titleParts.filter(Boolean).join(' ').slice(0, 32)
}

function buildSmartPostDescription(fields) {
  const source = fields || {}
  const category = trimText(source.category)
  const subCategory = trimText(source.subCategory)
  const timeInfo = parsePostTimeInfo(source)
  if (!category || !subCategory || !timeInfo) return ''
  const maxCount = parseInt(source.maxCount, 10) || 0
  return buildDescription({ category, subCategory }, timeInfo, trimText(source.locationText), maxCount)
}

function buildSmartPostDraft(input, history, options) {
  const text = trimText(input)
  const now = options && options.now ? new Date(options.now) : new Date()
  if (!text) {
    return { ok: false, error: '先输入一句话，例如：明天羽毛球' }
  }
  const activity = detectActivity(text)
  if (!activity) {
    return { ok: false, error: '还没识别出活动类型，可以试试：明天羽毛球 / 周五桌游 / 后天电影' }
  }

  const notes = []
  const currentLocation = options && options.currentLocation
  const relevantHistory = collectHistory(history || {}, activity, now)
  const historyClock = selectHistoryClock(relevantHistory)
  const historyRange = selectHistoryRange(relevantHistory)
  const historyMaxCount = selectHistoryMaxCount(relevantHistory)
  const explicitMaxCount = detectPeopleCount(text)
  const explicitLocation = detectExplicitLocation(text, activity)
  const historyLocation = selectHistoryLocation(relevantHistory.map((item) => Object.assign({}, item)), currentLocation)
  const resolvedTime = resolveTime({ text }, historyClock, historyRange, now, notes, options || {})
  if (resolvedTime && resolvedTime.error === 'PAST_DATE_NEEDS_CONFIRM') {
    return {
      ok: false,
      needConfirm: true,
      confirmType: 'pastDate',
      message: resolvedTime.message,
    }
  }
  const maxCount = explicitMaxCount || historyMaxCount || 2
  if (explicitMaxCount) {
    notes.push('人数使用了你输入里的 ' + explicitMaxCount + ' 人。')
  }

  let locationText = ''
  let locationCoords = null
  let locationMode = 'manual'
  if (explicitLocation) {
    locationText = explicitLocation
    notes.push('地点使用了你输入中明确提到的位置。')
  } else if (historyLocation.best) {
    locationText = historyLocation.best.address
    locationCoords = historyLocation.best.coords
    if (historyLocation.best.distanceKm !== null) {
      notes.push('历史地点距离当前位置约 ' + historyLocation.best.distanceKm.toFixed(1) + 'km，已用于补全地点。')
    } else {
      notes.push('地点参考了你历史同类活动的常用地点。')
    }
  } else if (currentLocation) {
    locationText = trimText(currentLocation.address) || '当前位置'
    locationCoords = { latitude: currentLocation.latitude, longitude: currentLocation.longitude }
    locationMode = 'current'
    notes.push('历史地点离当前位置较远或缺少坐标，地点改用当前位置。')
  }
  if (historyLocation.filteredFarCount > 0 && !historyLocation.best) {
    notes.push('有历史地点距离当前位置较远，已跳过这些地点。')
  }

  const titleParts = [shortTitleTime(resolvedTime), shortLocation(locationText), activity.subCategory + '局']
  const title = titleParts.filter(Boolean).join(' ').slice(0, 32)
  const description = buildDescription(activity, resolvedTime, locationText, maxCount)
  const fields = {
    title,
    description,
    category: activity.category,
    subCategory: activity.subCategory,
    timeMode: resolvedTime.mode,
    timeRange: resolvedTime.mode === 'range' ? resolvedTime.days : 7,
    fixedTime: resolvedTime.mode === 'fixed' ? resolvedTime.date.toISOString() : '',
    fixedTimeDisplay: resolvedTime.mode === 'fixed' ? formatFixedDisplay(resolvedTime.date) : '',
    selectedDate: resolvedTime.mode === 'fixed' ? formatDate(resolvedTime.date) : '',
    selectedClock: resolvedTime.mode === 'fixed' ? formatClock(resolvedTime.date.getHours(), resolvedTime.date.getMinutes()) : '09:00',
    locationMode,
    locationText,
    locationCoords,
    maxCount,
  }

  const summary = [
    '识别为：' + activity.category + ' / ' + activity.subCategory,
    '时间：' + timeLabel(resolvedTime),
    '人数：' + maxCount + ' 人',
  ]
  if (locationText) summary.push('地点：' + locationText)
  if (!relevantHistory.length) summary.push('没有找到同类历史活动，本次主要按默认模板生成。')
  notes.forEach((item) => summary.push(item))

  return {
    ok: true,
    fields,
    summary,
  }
}

module.exports = {
  buildSmartPostDraft,
  buildSmartPostTitle,
  buildSmartPostDescription,
}
