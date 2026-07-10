export function formatDateTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(Number(value));
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleString("zh-CN", {
    hour12: false,
  });
}

export function formatScore(value, digits = 1) {
  if (value === undefined || value === null || Number.isNaN(Number(value))) {
    return "-";
  }
  return Number(value).toFixed(digits);
}

export function formatDelta(value) {
  const number = Number(value || 0);
  if (number > 0) {
    return `+${number}`;
  }
  return `${number}`;
}
