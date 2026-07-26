import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { loginByPassword } from "../lib/api";
import { saveSession } from "../lib/session";

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const [form, setForm] = useState({ nickname: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const payload = await loginByPassword(form);
      if (payload?.user?.role !== "admin") {
        setError("当前账号不是管理员，不能进入后台。");
        return;
      }
      saveSession({
        token: payload.accessToken || payload.token,
        refreshToken: payload.refreshToken,
        user: payload.user,
      });
      navigate(location.state?.from || "/", { replace: true });
    } catch (err) {
      setError(err.message || "登录失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="login-shell">
      <form className="login-card" onSubmit={handleSubmit}>
        <div className="login-title">活动管理后台</div>
        <div className="login-subtitle">传统后台风格，支持争议案例处理、用户巡检、活动治理和管理员账号管理。</div>

        <label className="field">
          <span>管理员账号</span>
          <input
            value={form.nickname}
            onChange={(event) => setForm((prev) => ({ ...prev, nickname: event.target.value }))}
            placeholder="请输入管理员账号"
            autoComplete="username"
          />
        </label>

        <label className="field">
          <span>密码</span>
          <input
            type="password"
            value={form.password}
            onChange={(event) => setForm((prev) => ({ ...prev, password: event.target.value }))}
            placeholder="请输入密码"
            autoComplete="current-password"
          />
        </label>

        {error ? <div className="error-banner">{error}</div> : null}

        <button className="primary-button" disabled={submitting}>
          {submitting ? "登录中..." : "登录后台"}
        </button>
      </form>
    </div>
  );
}