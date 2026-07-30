import { FormEvent, useState } from "react";
import { api, ApiError, User } from "../api";
import { LangSwitch, useT } from "../i18n";

export default function Login({ onAuthed }: { onAuthed: (u: User) => void }) {
  const { t } = useT();
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
      setError(err instanceof ApiError ? err.message : t("auth.networkError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-wrap">
      <div style={{ position: "fixed", top: 14, right: 20 }}><LangSwitch /></div>
      <div className="auth-card">
        <div className="brand"><em>●</em> Stocker</div>
        <p className="auth-sub">{t("auth.sub")}</p>
        <form onSubmit={submit}>
          <input placeholder={t("auth.username")} value={username} autoComplete="username"
            onChange={e => setUsername(e.target.value)} />
          <input placeholder={t("auth.password")} type="password" autoComplete="current-password"
            value={password} onChange={e => setPassword(e.target.value)} />
          {error && <p className="form-error">{error}</p>}
          <button className="submit" disabled={busy || !username || !password}>
            {mode === "login" ? t("auth.login") : t("auth.register")}
          </button>
        </form>
        <button className="link-btn" onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(null); }}>
          {mode === "login" ? t("auth.toRegister") : t("auth.toLogin")}
        </button>
      </div>
    </div>
  );
}
