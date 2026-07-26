import { FormEvent, useState } from "react";
import { api, ApiError, User } from "../api";

export default function Login({ onAuthed }: { onAuthed: (u: User) => void }) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const u = await api.post<User>(`/api/${mode}`, { username, password });
      onAuthed(u);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "网络错误，请重试");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="brand"><em>●</em> Stocker</div>
        <p className="auth-sub">回到过去，和朋友重新炒一次那段历史。</p>
        <form onSubmit={submit}>
          <input placeholder="用户名" value={username} autoComplete="username"
            onChange={e => setUsername(e.target.value)} />
          <input placeholder="密码" type="password" autoComplete="current-password"
            value={password} onChange={e => setPassword(e.target.value)} />
          {error && <p className="form-error">{error}</p>}
          <button className="submit" disabled={busy || !username || !password}>
            {mode === "login" ? "登录" : "注册"}
          </button>
        </form>
        <button className="link-btn" onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(null); }}>
          {mode === "login" ? "注册新账号" : "已有账号，去登录"}
        </button>
      </div>
    </div>
  );
}
