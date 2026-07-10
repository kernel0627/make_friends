import { useEffect, useState } from "react";
import { ChartCard, SimpleBarChart, SimpleDonutChart, SimpleLineChart } from "../components/SimpleCharts";
import StatCard from "../components/StatCard";
import { fetchAdminDashboardAnalytics, fetchAdminDashboardSummary } from "../lib/api";
import { mapAnalyticsSeries, mapCaseStatusName, mapPostStatusName } from "../lib/display";

const windows = [
  { label: "7 天", value: "7d" },
  { label: "30 天", value: "30d" },
  { label: "90 天", value: "90d" },
];

export default function DashboardPage() {
  const [summary, setSummary] = useState(null);
  const [analytics, setAnalytics] = useState(null);
  const [windowKey, setWindowKey] = useState("30d");
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    fetchAdminDashboardSummary()
      .then((payload) => active && setSummary(payload))
      .catch((err) => active && setError(err.message || "加载仪表盘汇总失败"));
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    fetchAdminDashboardAnalytics({ window: windowKey })
      .then((payload) => active && setAnalytics(payload))
      .catch((err) => active && setError(err.message || "加载图表数据失败"));
    return () => {
      active = false;
    };
  }, [windowKey]);

  const stats = summary || {};

  return (
    <div className="dashboard-stack">
      <section className="page-grid dashboard-hero">
        <div className="page-header page-header-spacious">
          <div>
            <h1>仪表盘</h1>
            <p>先看整体风险和趋势，再进入争议处理、用户巡检和活动治理。</p>
          </div>
          <div className="segmented-control">
            {windows.map((item) => (
              <button
                key={item.value}
                className={`segmented-button${windowKey === item.value ? " segmented-button-active" : ""}`}
                onClick={() => setWindowKey(item.value)}
              >
                {item.label}
              </button>
            ))}
          </div>
        </div>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="stats-grid stats-grid-spacious">
          <StatCard label="待处理案例" value={stats.openCases ?? "-"} tone="danger" />
          <StatCard label="处理中案例" value={stats.inReviewCases ?? "-"} tone="warn" />
          <StatCard label="争议履约" value={stats.disputedSettlements ?? "-"} tone="warn" />
          <StatCard label="待完成评价" value={stats.pendingReviews ?? "-"} />
          <StatCard label="近 7 天信誉流水" value={stats.recentCreditDeltas ?? "-"} />
          <StatCard label="总用户数" value={stats.totalUsers ?? "-"} />
          <StatCard label="总活动数" value={stats.totalPosts ?? "-"} />
          <StatCard label="已关闭活动" value={stats.closedPosts ?? "-"} />
        </div>
      </section>

      <div className="dashboard-chart-grid">
        <ChartCard title="用户注册趋势" subtitle="按时间窗口观察新用户的进入速度。">
          <SimpleLineChart series={analytics?.dailyUsers || []} stroke="#2f6af6" />
        </ChartCard>

        <ChartCard title="活动发布趋势" subtitle="看清最近活动创建的节奏和波峰。">
          <SimpleLineChart series={analytics?.dailyPosts || []} stroke="#e98b2a" />
        </ChartCard>

        <ChartCard title="争议案例趋势" subtitle="争议数量如果突然抬头，通常意味着履约或运营侧有问题。">
          <SimpleLineChart series={analytics?.dailyCases || []} stroke="#d04c4c" />
        </ChartCard>

        <ChartCard title="信誉流水趋势" subtitle="观察信誉加减分是否过度集中，判断规则是否平衡。">
          <SimpleLineChart series={analytics?.dailyCreditDeltas || []} stroke="#6d5bd0" />
        </ChartCard>

        <ChartCard title="活动分类占比" subtitle="看当前平台主要被哪些活动类型占据。">
          <SimpleDonutChart series={analytics?.categoryDistribution || []} />
        </ChartCard>

        <ChartCard title="状态分布" subtitle="同时看案例和活动状态，便于快速判断当前风险压力。">
          <div className="chart-split">
            <div>
              <div className="mini-chart-title">案例状态</div>
              <SimpleBarChart series={mapAnalyticsSeries(analytics?.caseStatusDistribution || [], mapCaseStatusName)} color="#d04c4c" />
            </div>
            <div>
              <div className="mini-chart-title">活动状态</div>
              <SimpleBarChart series={mapAnalyticsSeries(analytics?.postStatusDistribution || [], mapPostStatusName)} color="#2f6af6" />
            </div>
          </div>
        </ChartCard>

        <ChartCard title="热门子类 Top" subtitle="快速识别最近最活跃的活动子类。">
          <SimpleBarChart series={analytics?.topSubCategories || []} color="#13a38b" />
        </ChartCard>
      </div>
    </div>
  );
}