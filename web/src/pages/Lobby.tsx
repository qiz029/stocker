import { FormEvent, useEffect, useState } from "react";
import type { CSSProperties, KeyboardEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, Room, ScenarioInfo } from "../api";
import { LangSwitch, TFunc, pickL, useT } from "../i18n";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUser } from "../App";
import DocsLink from "../components/DocsLink";
import nifty1972 from "../assets/eras/nifty-1972.avif";
import crash1987 from "../assets/eras/crash-1987.avif";
import dotcom2000 from "../assets/eras/dotcom-2000.avif";
import gfc2008 from "../assets/eras/gfc-2008.avif";

type EraVisual = { year: number; image: string; accent: string };

const eraVisuals: Record<string, EraVisual> = {
  "nifty-1972": { year: 1972, image: nifty1972, accent: "#efb84a" },
  "crash-1987": { year: 1987, image: crash1987, accent: "#ff463d" },
  "dotcom-2000": { year: 2000, image: dotcom2000, accent: "#9d6cff" },
  "gfc-2008": { year: 2008, image: gfc2008, accent: "#52c7d9" },
};

function scenarioYear(sc: ScenarioInfo): number {
  const known = eraVisuals[sc.id]?.year;
  if (known) return known;
  const match = `${sc.id} ${sc.name}`.match(/(?:19|20)\d{2}/);
  return match ? Number(match[0]) : Number.MAX_SAFE_INTEGER;
}

function chronologicalScenarios(items: ScenarioInfo[]): ScenarioInfo[] {
  return [...items].sort((a, b) => scenarioYear(a) - scenarioYear(b) || a.name.localeCompare(b.name));
}

function eraTitle(sc: ScenarioInfo, lang: "en" | "zh"): string {
  return pickL(lang, sc.name, sc.name_en).replace(/^\s*(?:19|20)\d{2}\s*[·—:-]?\s*/, "");
}

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
  const { t, lang } = useT();
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
        const items = chronologicalScenarios(res.items ?? []); // defensive: older mocks/tests may omit it
        setScenarios(items);
        if (items.length) setScenarioID(current => current || items[0]!.id);
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

  function selectScenario(id: string) {
    setScenarioID(id);
    setDuration(0);
  }

  function navigateEras(e: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (index + 1) % scenarios.length;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (index - 1 + scenarios.length) % scenarios.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = scenarios.length - 1;
    else return;

    e.preventDefault();
    selectScenario(scenarios[next]!.id);
    e.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[next]?.focus();
  }

  function roomStatus(r: Room): { tag: string; cls: string } {
    if (r.status === "lobby") return { tag: t("status.waiting"), cls: "done" };
    if (r.ended) return { tag: t("status.ended"), cls: "done" };
    return { tag: t("status.running"), cls: "live" };
  }

  const datedYears = scenarios.map(scenarioYear).filter(year => year !== Number.MAX_SAFE_INTEGER);

  return (
    <div className="wrap lobby">
      <div className="topbar" style={{ margin: "-22px -20px 22px" }}>
        <div className="brand"><em>●</em> Stocker</div>
        <div className="spacer" />
        <DocsLink />
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
        <div className="create-room-panel">
          <div className="era-picker-head">
            <div>
              <span className="era-kicker">{t("lobby.chooseEra")}</span>
              <p>{t("lobby.chooseEraHint")}</p>
            </div>
            {datedYears.length > 1 && (
              <span className="era-direction" aria-hidden="true">{datedYears[0]} <i /> {datedYears.at(-1)}</span>
            )}
          </div>

          <div className="era-timeline" role="radiogroup" aria-label={t("lobby.chooseEra")}
            data-era-count={scenarios.length}
            style={{ "--era-count": Math.max(scenarios.length, 1) } as CSSProperties}>
            {scenarios.map((sc, index) => {
              const visual = eraVisuals[sc.id];
              const year = scenarioYear(sc);
              const selected = sc.id === scenarioID;
              return (
                <button key={sc.id} type="button" role="radio" aria-checked={selected}
                  tabIndex={selected ? 0 : -1}
                  data-era-year={year === Number.MAX_SAFE_INTEGER ? undefined : year}
                  className={`era-card${selected ? " selected" : ""}`}
                  style={{ "--era-accent": visual?.accent ?? "#00c805" } as CSSProperties}
                  onClick={() => selectScenario(sc.id)} onKeyDown={e => navigateEras(e, index)}>
                  {index < scenarios.length - 1 && <span className="era-link" aria-hidden="true" />}
                  <span className="era-node" aria-hidden="true" />
                  {visual && <img className="era-card-art" src={visual.image} alt="" aria-hidden="true"
                    loading="lazy" decoding="async" width="800" height="534" />}
                  <span className="era-card-shade" aria-hidden="true" />
                  <span className="era-card-index num">{String(index + 1).padStart(2, "0")}</span>
                  <span className="era-card-copy">
                    <strong className="era-year num">{year === Number.MAX_SAFE_INTEGER ? "ALT" : year}</strong>
                    <span className="era-title">{eraTitle(sc, lang)}</span>
                    <span className="era-days">{t("lobby.tradingDays", { days: sc.days })}</span>
                  </span>
                  <span className="era-card-state">{selected ? t("lobby.selectedEra") : t("lobby.selectEra")}</span>
                </button>
              );
            })}
          </div>

          <div className="create-room-actions">
            <label className="duration-field">
              <span>{t("lobby.gameDuration")}</span>
              <select aria-label={t("lobby.gameDuration")}
                value={duration || durationOptions(scenarios.find(sc => sc.id === scenarioID)?.days ?? 300, t)[1]![1]}
                onChange={e => setDuration(Number(e.target.value))}>
                {durationOptions(scenarios.find(sc => sc.id === scenarioID)?.days ?? 300, t).map(([label, secs]) => (
                  <option key={secs} value={secs}>{label}</option>
                ))}
              </select>
            </label>
            <button className="submit create-era-submit"
              onClick={create} disabled={busy || !scenarioID}>{busy ? t("lobby.creating") : t("lobby.create")}</button>
          </div>
        </div>
      ) : (
        <button className="ghost-btn" onClick={() => setShowCreate(true)}>{t("lobby.newRoom")}</button>
      )}
      {node}
    </div>
  );
}
