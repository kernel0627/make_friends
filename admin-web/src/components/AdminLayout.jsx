import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { clearSession, getSession } from "../lib/session";

const navItems = [
  { to: "/", label: "仪表盘" },
  { to: "/cases", label: "争议案例" },
  { to: "/users", label: "用户管理" },
  { to: "/posts", label: "活动管理" },
  { to: "/admins", label: "管理员账号" },
];

export default function AdminLayout() {
  const navigate = useNavigate();
  const session = getSession();
  const user = session?.user || {};

  function handleLogout() {
    clearSession();
    navigate("/login", { replace: true });
  }

  return (
    <div className="admin-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <div className="sidebar-title">活动管理后台</div>
          <div className="sidebar-subtitle">查看项目风险、处理争议案例、管理用户和活动数据。</div>
        </div>
        <nav className="sidebar-nav">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) => `sidebar-link${isActive ? " sidebar-link-active" : ""}`}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="main-shell">
        <header className="topbar">
          <div>
            <div className="topbar-title">后台总览</div>
            <div className="topbar-subtitle">左侧导航固定，内容区和详情区各自滚动，方便持续处理数据。</div>
          </div>
          <div className="topbar-user">
            <div className="topbar-user-meta">
              <div className="topbar-user-name">{user.nickName || user.nickname || "管理员"}</div>
              <div className="topbar-subtitle">{user.rootAdmin ? "最高管理员" : "普通管理员"}</div>
            </div>
            <button className="ghost-button" onClick={handleLogout}>
              退出登录
            </button>
          </div>
        </header>
        <main className="page-shell">
          <Outlet />
        </main>
      </div>
    </div>
  );
}