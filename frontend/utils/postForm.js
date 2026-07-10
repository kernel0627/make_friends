function validatePostForm(form, options) {
  const opts = options || {}
  const errors = {}
  const title = (form.title || '').trim()
  const category = (form.category || '').trim()
  const subCategory = (form.subCategory || '').trim()
  const timeMode = form.timeMode || 'range'
  const timeRange = parseInt(form.timeRange, 10)
  const fixedTime = form.fixedTime
  const locationText = (form.locationText || '').trim()
  const maxCount = parseInt(form.maxCount, 10)
  const minMaxCount = opts.minMaxCount || 2
  const requireSubCategory = !!opts.requireSubCategory

  if (!title) {
    errors.title = '请输入活动标题'
  }
  if (!category) {
    errors.category = '请选择活动分类'
  }
  if (requireSubCategory && !subCategory) {
    errors.subCategory = '请选择具体类型'
  }

  if (timeMode === 'range') {
    if (!Number.isFinite(timeRange) || timeRange < 1 || timeRange > 30) {
      errors.timeRange = '范围天数需要在 1 到 30 天之间'
    }
  } else {
    if (!fixedTime) {
      errors.fixedTime = '请选择固定时间'
    } else {
      const fixedTs = Date.parse(fixedTime)
      if (!Number.isFinite(fixedTs)) {
        errors.fixedTime = '固定时间格式不正确'
      } else if (fixedTs <= Date.now()) {
        errors.fixedTime = '固定时间必须晚于当前时间'
      }
    }
  }

  if (!locationText) {
    errors.locationText = '请输入活动地点'
  }
  if (!Number.isFinite(maxCount) || maxCount < minMaxCount) {
    errors.maxCount = '人数上限不能小于 ' + minMaxCount + ' 人'
  }

  return { valid: Object.keys(errors).length === 0, errors }
}

function buildTimeInfo(timeMode, timeRange, fixedTime) {
  if (timeMode === 'range') {
    return { mode: 'range', days: parseInt(timeRange, 10) || 1 }
  }
  return { mode: 'fixed', fixedTime }
}

module.exports = {
  validatePostForm,
  buildTimeInfo,
}