function normalizeSeries(series = []) {
  return Array.isArray(series) ? series.filter((item) => item && item.label) : [];
}

export function ChartCard({ title, subtitle, children, actions }) {
  return (
    <section className="panel chart-panel">
      <div className="panel-header">
        <div>
          <h2>{title}</h2>
          {subtitle ? <p>{subtitle}</p> : null}
        </div>
        {actions ? <div className="panel-actions">{actions}</div> : null}
      </div>
      {children}
    </section>
  );
}

export function SimpleLineChart({ series = [], stroke = "#2557c4" }) {
  const data = normalizeSeries(series);
  if (!data.length) {
    return <div className="chart-empty">暂无趋势数据</div>;
  }

  const width = 680;
  const height = 260;
  const padding = 28;
  const chartHeight = height - padding * 2;
  const stepX = data.length > 1 ? (width - padding * 2) / (data.length - 1) : 0;
  const maxValue = Math.max(...data.map((item) => Number(item.value) || 0), 1);
  const points = data.map((item, index) => {
    const value = Number(item.value) || 0;
    const x = padding + index * stepX;
    const y = height - padding - (value / maxValue) * chartHeight;
    return { x, y, label: item.label, value };
  });
  const path = points.map((point, index) => `${index === 0 ? "M" : "L"}${point.x},${point.y}`).join(" ");

  return (
    <div className="chart-wrap">
      <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg" role="img">
        <defs>
          <linearGradient id={`line-fill-${stroke.replace("#", "")}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={stroke} stopOpacity="0.22" />
            <stop offset="100%" stopColor={stroke} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        {[0, 0.25, 0.5, 0.75, 1].map((ratio) => {
          const y = height - padding - chartHeight * ratio;
          return <line key={ratio} x1={padding} x2={width - padding} y1={y} y2={y} className="chart-grid-line" />;
        })}
        <path d={`${path} L ${width - padding},${height - padding} L ${padding},${height - padding} Z`} fill={`url(#line-fill-${stroke.replace("#", "")})`} />
        <path d={path} fill="none" stroke={stroke} strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        {points.map((point) => (
          <g key={point.label}>
            <circle cx={point.x} cy={point.y} r="4.5" fill={stroke} />
          </g>
        ))}
      </svg>
      <div className="chart-axis-labels">
        {data.map((item) => (
          <span key={item.label}>{item.label}</span>
        ))}
      </div>
    </div>
  );
}

export function SimpleBarChart({ series = [], color = "#2d7cf7" }) {
  const data = normalizeSeries(series);
  if (!data.length) {
    return <div className="chart-empty">暂无分布数据</div>;
  }
  const maxValue = Math.max(...data.map((item) => Number(item.value) || 0), 1);
  return (
    <div className="bars">
      {data.map((item) => {
        const value = Number(item.value) || 0;
        const width = `${Math.max((value / maxValue) * 100, 4)}%`;
        return (
          <div className="bar-row" key={item.label}>
            <div className="bar-label">{item.label}</div>
            <div className="bar-track">
              <div className="bar-fill" style={{ width, background: color }} />
            </div>
            <div className="bar-value">{value}</div>
          </div>
        );
      })}
    </div>
  );
}

export function SimpleDonutChart({ series = [] }) {
  const data = normalizeSeries(series);
  if (!data.length) {
    return <div className="chart-empty">暂无占比数据</div>;
  }
  const total = data.reduce((sum, item) => sum + (Number(item.value) || 0), 0) || 1;
  const radius = 72;
  const circumference = 2 * Math.PI * radius;
  const palette = ["#2557c4", "#13a38b", "#ee8f00", "#8a63d2", "#d1495b", "#2b9ed6", "#7a889f"];
  let offset = 0;

  return (
    <div className="donut-layout">
      <svg viewBox="0 0 220 220" className="donut-svg" role="img">
        <circle cx="110" cy="110" r={radius} className="donut-bg" />
        {data.map((item, index) => {
          const value = Number(item.value) || 0;
          const ratio = value / total;
          const dash = ratio * circumference;
          const strokeDasharray = `${dash} ${circumference - dash}`;
          const currentOffset = offset;
          offset += dash;
          return (
            <circle
              key={item.label}
              cx="110"
              cy="110"
              r={radius}
              className="donut-segment"
              stroke={palette[index % palette.length]}
              strokeDasharray={strokeDasharray}
              strokeDashoffset={-currentOffset}
            />
          );
        })}
        <text x="110" y="104" textAnchor="middle" className="donut-total-label">总量</text>
        <text x="110" y="128" textAnchor="middle" className="donut-total-value">{total}</text>
      </svg>
      <div className="donut-legend">
        {data.map((item, index) => (
          <div className="legend-row" key={item.label}>
            <span className="legend-dot" style={{ background: palette[index % palette.length] }} />
            <span className="legend-label">{item.label}</span>
            <span className="legend-value">{Number(item.value) || 0}</span>
          </div>
        ))}
      </div>
    </div>
  );
}