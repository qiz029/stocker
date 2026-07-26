import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, Room } from "../api";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUser } from "../App";

const DURATIONS: [string, number][] = [
  ["1 周局（约 34 分钟/交易日）", 2016],
  ["2 周局（约 67 分钟/交易日）", 4032],
  ["4 周局（约 134 分钟/交易日）", 8064],
  ["测试局（1 分钟/交易日）", 60],
];

export default function Lobby() {
  const user = useUser();
  const navigate = useNavigate();
  const { toast, node } = useToast();
  const { data, reload } = usePoll(() => api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const [invite, setInvite] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [duration, setDuration] = useState(4032);
  const [busy, setBusy] = useState(false);

  async function join(e: FormEvent) {
    e.preventDefault();
    try {
      const room = await api.post<Room>("/api/rooms/join", { invite_code: invite.trim() });
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "加入失败");
    }
  }

  async function create() {
    setBusy(true);
    try {
      const room = await api.post<Room>("/api/rooms", {
        scenario_id: "synthetic-v1", day_duration_secs: duration,
      });
      toast("平行世界生成完毕");
      void reload();
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "创建失败");
    } finally {
      setBusy(false);
    }
  }

  function roomStatus(r: Room): { tag: string; cls: string } {
    if (r.status === "lobby") return { tag: "等待开局", cls: "done" };
    if (r.ended) return { tag: "已结束", cls: "done" };
    return { tag: "进行中", cls: "live" };
  }

  return (
    <div className="wrap lobby">
      <div className="topbar" style={{ margin: "-22px -20px 22px" }}>
        <div className="brand"><em>●</em> Stocker</div>
        <div className="spacer" />
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>
      <h1>我的房间</h1>
      <p className="sub">和朋友回到过去，重新炒一次那段历史。</p>

      {(data?.rooms ?? []).map(r => {
        const st = roomStatus(r);
        return (
          <div key={r.id} className="room-card" onClick={() => navigate(`/rooms/${r.id}`)}>
            <div className="rc-top">
              <h3>神秘年代 #{r.id}</h3>
              <span className={`tag ${st.cls}`}>{st.tag}</span>
            </div>
            <div className="rc-meta">
              {r.status === "running" && r.current_day !== undefined
                ? <>第 <b className="num">{r.current_day}</b> / {r.days} 个交易日</>
                : <>邀请码 <b className="num">{r.invite_code}</b> · 每交易日 {Math.round(r.day_duration_secs / 60)} 分钟</>}
            </div>
            {r.status === "running" && r.current_day !== undefined && (
              <div className="progress"><i style={{ width: `${(r.current_day / r.days) * 100}%` }} /></div>
            )}
          </div>
        );
      })}

      <form className="lobby-form" onSubmit={join}>
        <input placeholder="输入邀请码" value={invite} onChange={e => setInvite(e.target.value)} />
        <button className="submit" style={{ width: "auto", padding: "10px 22px" }} disabled={!invite.trim()}>加入</button>
      </form>

      {showCreate ? (
        <div className="lobby-form">
          <select value={duration} onChange={e => setDuration(Number(e.target.value))}>
            {DURATIONS.map(([label, secs]) => <option key={secs} value={secs}>{label}</option>)}
          </select>
          <button className="submit" style={{ width: "auto", padding: "10px 22px" }}
            onClick={create} disabled={busy}>{busy ? "生成平行世界…" : "创建"}</button>
        </div>
      ) : (
        <button className="ghost-btn" onClick={() => setShowCreate(true)}>＋ 创建新房间</button>
      )}
      {node}
    </div>
  );
}
