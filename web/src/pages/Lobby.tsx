import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, Room, ScenarioInfo } from "../api";
import { LangSwitch, TFunc, useT } from "../i18n";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUser } from "../App";

function durationOptions(days: number, t: TFunc): [string, number][] {
  const opts: [string, number][] = [1, 2, 4].map(weeks => {
    const secs = Math.max(60, Math.round((weeks * 604800) / days));
    return [t("lobby.durationWeeks", { weeks, mins: Math.max(1, Math.round(secs / 60)) }), secs];
  });
  opts.push([t("lobby.durationTest"), 60]);
  return opts;
}

export default function Lobby() {
  const user = useUser();
  const { t } = useT();
  const navigate = useNavigate();
  const { toast, node } = useToast();
  const { data, reload } = usePoll(() => api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const [invite, setInvite] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [scenarioID, setScenarioID] = useState("");
  const [duration, setDuration] = useState(0);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get<{ items: ScenarioInfo[] }>("/api/scenarios")
      .then(res => {
        const items = res.items ?? []; // defensive: older mocks/tests may omit it
        setScenarios(items);
        if (items.length && !scenarioID) setScenarioID(items[0]!.id);
      })
      .catch(() => setScenarios([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function join(e: FormEvent) {
    e.preventDefault();
    try {
      const room = await api.post<Room>("/api/rooms/join", { invite_code: invite.trim() });
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("lobby.joinFailed"));
    }
  }

  async function create() {
    setBusy(true);
    try {
      const currentScenario = scenarios.find(sc => sc.id === scenarioID);
      const finalDuration = duration || durationOptions(currentScenario?.days ?? 300, t)[1]![1];
      const room = await api.post<Room>("/api/rooms", {
        scenario_id: scenarioID, day_duration_secs: finalDuration,
      });
      toast(t("lobby.created"));
      void reload();
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("lobby.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  function roomStatus(r: Room): { tag: string; cls: string } {
    if (r.status === "lobby") return { tag: t("status.waiting"), cls: "done" };
    if (r.ended) return { tag: t("status.ended"), cls: "done" };
    return { tag: t("status.running"), cls: "live" };
  }

  return (
    <div className="wrap lobby">
      <div className="topbar" style={{ margin: "-22px -20px 22px" }}>
        <div className="brand"><em>●</em> Stocker</div>
        <div className="spacer" />
        <LangSwitch />
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>
      <h1>{t("lobby.title")}</h1>
      <p className="sub">{t("lobby.sub")}</p>

      {(data?.rooms ?? []).map(r => {
        const st = roomStatus(r);
        return (
          <div key={r.id} className="room-card" onClick={() => navigate(`/rooms/${r.id}`)}>
            <div className="rc-top">
              <h3>{t("era.name")} #{r.id}</h3>
              <span className={`tag ${st.cls}`}>{st.tag}</span>
            </div>
            <div className="rc-meta">
              {r.status === "running" && r.current_day !== undefined
                ? <>{t("lobby.dayA")} <b className="num">{r.current_day}</b> {t("lobby.dayB", { days: r.days })}</>
                : <>{t("lobby.inviteA")} <b className="num">{r.invite_code}</b> {t("lobby.inviteB", { mins: Math.round(r.day_duration_secs / 60) })}</>}
            </div>
            {r.status === "running" && r.current_day !== undefined && (
              <div className="progress"><i style={{ width: `${(r.current_day / r.days) * 100}%` }} /></div>
            )}
          </div>
        );
      })}

      <form className="lobby-form" onSubmit={join}>
        <input placeholder={t("lobby.joinPlaceholder")} value={invite} onChange={e => setInvite(e.target.value)} />
        <button className="submit" style={{ width: "auto", padding: "10px 22px" }} disabled={!invite.trim()}>{t("lobby.join")}</button>
      </form>

      {showCreate ? (
        <div className="lobby-form">
          <select value={scenarioID} onChange={e => { setScenarioID(e.target.value); setDuration(0); }}>
            {scenarios.map(sc => <option key={sc.id} value={sc.id}>{sc.name}{t("lobby.scenarioDays", { days: sc.days })}</option>)}
          </select>
          <select value={duration || durationOptions(scenarios.find(sc => sc.id === scenarioID)?.days ?? 300, t)[1]![1]}
            onChange={e => setDuration(Number(e.target.value))}>
            {durationOptions(scenarios.find(sc => sc.id === scenarioID)?.days ?? 300, t).map(([label, secs]) => (
              <option key={secs} value={secs}>{label}</option>
            ))}
          </select>
          <button className="submit" style={{ width: "auto", padding: "10px 22px" }}
            onClick={create} disabled={busy || !scenarioID}>{busy ? t("lobby.creating") : t("lobby.create")}</button>
        </div>
      ) : (
        <button className="ghost-btn" onClick={() => setShowCreate(true)}>{t("lobby.newRoom")}</button>
      )}
      {node}
    </div>
  );
}
