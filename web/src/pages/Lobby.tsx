import { FormEvent, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, AvatarID, EraLeader, PublicRoom, Room, ScenarioInfo, User } from "../api";
import { LangSwitch, pickL, useT } from "../i18n";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUpdateUser, useUser } from "../App";
import DocsLink from "../components/DocsLink";
import nifty1972 from "../assets/eras/nifty-1972.avif";
import crash1987 from "../assets/eras/crash-1987.avif";
import dotcom2000 from "../assets/eras/dotcom-2000.avif";
import gfc2008 from "../assets/eras/gfc-2008.avif";
import "./LobbyHall.css";
import { avatarGlyph, avatarGlyphs, avatarIDs } from "../avatar";

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
  const match = (sc.id + " " + sc.name).match(/(?:19|20)\d{2}/);
  return match ? Number(match[0]) : Number.MAX_SAFE_INTEGER;
}
function chronologicalScenarios(items: ScenarioInfo[]) {
  return [...items].sort((a, b) => scenarioYear(a) - scenarioYear(b) || a.name.localeCompare(b.name));
}
function fmtReturn(value?: number) {
  if (value === undefined) return "—";
  return (value >= 0 ? "+" : "") + (value * 100).toFixed(1) + "%";
}
function yearLabel(sc: ScenarioInfo) {
  const year = scenarioYear(sc);
  return year === Number.MAX_SAFE_INTEGER ? "ALT" : String(year);
}
function durationOptions(days: number, lang: "en" | "zh"): [string, number][] {
  const result: [string, number][] = [1, 2, 4].map(weeks => {
    const secs = Math.max(60, Math.round((weeks * 604800) / days));
    return [lang === "zh" ? `${weeks} 周（每交易日约 ${Math.round(secs / 60)} 分钟）` : `${weeks} weeks (~${Math.round(secs / 60)} min/day)`, secs];
  });
  result.push([lang === "zh" ? "测试局（每交易日 1 分钟）" : "Test game (1 min/day)", 60]);
  return result;
}

type HallCopy = typeof hallCopy.en;
const hallCopy = {
  en: {
    title: "Market Hall", sub: "Find a table, watch history unfold, or open a timeline of your own.",
    rooms: "Public rooms", roomsSub: "Live and open tables across every era", board: "Era leaderboard",
    boardSub: "Best completed public runs — human players only", mine: "Your rooms", create: "＋ Create new room",
    invite: "Enter invite code", join: "Join", watch: "Watch", waiting: "WAITING", live: "LIVE", ended: "ENDED",
    players: "players", agent: "Agent ×5", active: "active rooms", online: "human seats", eras: "eras",
    all: "All eras", empty: "No public rooms yet. Create the first table.", wins: "wins", room: "Room",
    profileTitle: "Choose your player identity", profileSub: "A display name and avatar are required before you take a seat. You can spectate without joining.",
    displayName: "Display name", avatar: "Avatar", cancel: "Keep watching", saveJoin: "Save & join",
    saveContinue: "Save & continue",
    createTitle: "Open a public timeline", createSub: "Players can spectate immediately. Waiting rooms allow them to complete a profile and join.",
    era: "Choose an era", duration: "Game duration", createNow: "Create", creating: "Generating parallel world…",
  },
  zh: {
    title: "时代大厅", sub: "找张桌子，看历史重演，或者开启你自己的时间线。",
    rooms: "公开房间", roomsSub: "跨时代正在进行和等待加入的牌桌", board: "时代排行榜",
    boardSub: "公开对局最佳战绩，仅统计真人玩家", mine: "你的房间", create: "＋ 创建新房间",
    invite: "输入邀请码", join: "加入", watch: "围观", waiting: "等待开局", live: "正在进行", ended: "已结束",
    players: "名玩家", agent: "Agent ×5", active: "个活跃房间", online: "个真人席位", eras: "个时代",
    all: "全部时代", empty: "还没有公开房间，来开第一桌吧。", wins: "次冠军", room: "房间",
    profileTitle: "设置玩家身份", profileSub: "入座前需要填写显示名称并选择头像；不加入也可以继续围观。",
    displayName: "显示名称", avatar: "头像", cancel: "继续围观", saveJoin: "保存并加入",
    saveContinue: "保存并继续",
    createTitle: "开启公开时间线", createSub: "所有人可以直接围观；等待开局时，完成资料即可加入。",
    era: "选择时代", duration: "游戏时长", createNow: "创建", creating: "正在生成平行世界…",
  },
};

function HallRoomRow({ room, scenarios, c, lang, onWatch, onJoin, dense = false }: {
  room: PublicRoom | Room; scenarios: ScenarioInfo[]; c: HallCopy;
  lang: "en" | "zh"; onWatch: () => void; onJoin?: () => void; dense?: boolean;
}) {
  const sc = scenarios.find(item => item.id === room.scenario_id);
  const visual = eraVisuals[room.scenario_id];
  const year = sc ? scenarioYear(sc) : visual?.year;
  const running = room.status === "running" && !room.ended;
  const status = room.ended ? c.ended : running ? c.live : c.waiting;
  const publicRoom = room as PublicRoom;
  const progress = Math.round(((room.current_day ?? 0) / Math.max(1, room.days)) * 100);
  return (
    <article className="hall-room" style={{ "--room-accent": visual?.accent ?? "#00c805" } as React.CSSProperties}>
      <div className="hall-room-era" style={{ backgroundImage: visual ? `linear-gradient(90deg,transparent,#11151c),url(${visual.image})` : undefined }}>
        <b>{year && year !== Number.MAX_SAFE_INTEGER ? year : "ALT"}</b><span>{status}</span>
      </div>
      <div>
        <div className="hall-room-title"><strong>{c.room} #{room.id}</strong><span className={`hall-status ${running ? "live" : ""}`}><i />{status}</span></div>
        <div className="hall-room-meta">
          <span>◉ {publicRoom.human_players ?? 1}/{publicRoom.max_human_players ?? 12} {c.players}</span>
          <span className="hall-agent">{c.agent}</span>
          {sc && <span>{pickL(lang, sc.name, sc.name_en)}</span>}
        </div>
        {running && <div className="hall-progress"><i style={{ width: progress + "%" }} /></div>}
      </div>
      <div className="hall-room-move"><b>{fmtReturn(publicRoom.leader_return)}</b><span>{running ? `${lang === "zh" ? "第" : "Day "}${room.current_day ?? 0}/${room.days}${lang === "zh" ? "日" : ""}` : status}</span></div>
      <button className="hall-room-watch" aria-label={`${c.watch} ${c.room} #${room.id}`} onClick={onWatch} />
      {!dense && <button className="hall-room-action" onClick={e => { e.stopPropagation(); running || room.ended ? onWatch() : onJoin?.(); }}>{running || room.ended ? c.watch : c.join} →</button>}
    </article>
  );
}

export default function Lobby() {
  const account = useUser();
  const updateAccount = useUpdateUser();
  const { lang, t } = useT();
  const c = hallCopy[lang];
  const navigate = useNavigate();
  const { toast, node } = useToast();
  const { data: mine, reload: reloadMine } = usePoll(() => api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const { data: publicData, reload: reloadPublic } = usePoll(() => api.get<{ rooms: PublicRoom[] }>("/api/rooms/public"), 30_000, []);
  const { data: boardData } = usePoll(() => api.get<{ items: EraLeader[] }>("/api/leaderboards/eras"), 60_000, []);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [filter, setFilter] = useState<string>("all");
  const [boardEra, setBoardEra] = useState<string>("all");
  const [invite, setInvite] = useState("");
  const [dialog, setDialog] = useState<"profile" | "create" | null>(null);
  const [pendingRoom, setPendingRoom] = useState<PublicRoom | null>(null);
  const [pendingInvite, setPendingInvite] = useState("");
  const [pendingCreate, setPendingCreate] = useState(false);
  const [profileUser, setProfileUser] = useState<User>(account);
  const [displayName, setDisplayName] = useState(account.display_name ?? "");
  const [avatarID, setAvatarID] = useState<AvatarID>(account.avatar_id ?? "bull");
  const [scenarioID, setScenarioID] = useState("");
  const [duration, setDuration] = useState(0);
  const [busy, setBusy] = useState(false);

  useEffect(() => { api.get<{ items: ScenarioInfo[] }>("/api/scenarios").then(res => {
    const items = chronologicalScenarios(res.items ?? []); setScenarios(items); if (items[0]) setScenarioID(current => current || items[0]!.id);
  }).catch(() => setScenarios([])); }, []);
  useEffect(() => {
    if (!dialog) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (dialog === "profile") closeProfile(); else setDialog(null);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [dialog]);

  const publicRooms = (publicData?.rooms ?? []).filter(room => filter === "all" || room.scenario_id === filter);
  const leaders = (boardData?.items ?? []).filter(row => boardEra === "all" || row.scenario_id === boardEra).slice(0, 8);
  const activeRooms = (publicData?.rooms ?? []).filter(room => room.status === "running" && !room.ended).length;
  const humanSeats = (publicData?.rooms ?? []).reduce((sum, room) => sum + (room.human_players ?? 1), 0);
  const mostActive = useMemo(() => scenarios.map(sc => ({ sc, count: (publicData?.rooms ?? []).filter(r => r.scenario_id === sc.id).length })).sort((a,b) => b.count-a.count)[0], [publicData, scenarios]);

  async function joinInvite(e: FormEvent) {
    e.preventDefault();
    const code = invite.trim();
    if (!profileUser.profile_complete) {
      setPendingInvite(code); setPendingCreate(false); setPendingRoom(null); setDialog("profile"); return;
    }
    try { const room = await api.post<Room>("/api/rooms/join", { invite_code: code }); navigate(`/rooms/${room.id}`); }
    catch (err) {
      if (err instanceof ApiError && err.status === 422) { setPendingInvite(code); setDialog("profile"); }
      else toast(err instanceof ApiError ? err.message : t("lobby.joinFailed"));
    }
  }
  function requestJoin(room: PublicRoom) {
    if (profileUser.profile_complete) { void joinPublic(room); return; }
    setPendingRoom(room); setDialog("profile");
  }
  async function joinPublic(room: PublicRoom) {
    setBusy(true);
    try { await api.post(`/api/rooms/${room.id}/join`); await Promise.all([reloadMine(), reloadPublic()]); navigate(`/rooms/${room.id}`); }
    catch (err) { toast(err instanceof ApiError ? err.message : t("lobby.joinFailed")); }
    finally { setBusy(false); }
  }
  async function saveProfileAndJoin(e: FormEvent) {
    e.preventDefault(); setBusy(true);
    try {
      const updated = await api.put<User>("/api/me/profile", { display_name: displayName.trim(), avatar_id: avatarID });
      setProfileUser(updated); updateAccount(updated);
      const targetRoom = pendingRoom; const targetInvite = pendingInvite; const shouldCreate = pendingCreate;
      setPendingRoom(null); setPendingInvite(""); setPendingCreate(false);
      if (targetRoom) await joinPublic(targetRoom);
      else if (targetInvite) {
        const room = await api.post<Room>("/api/rooms/join", { invite_code: targetInvite });
        navigate(`/rooms/${room.id}`);
      } else if (shouldCreate) setDialog("create");
      else setDialog(null);
    } catch (err) { toast(err instanceof ApiError ? err.message : t("lobby.joinFailed")); }
    finally { setBusy(false); }
  }
  function openCreate() {
    if (profileUser.profile_complete) { setDialog("create"); return; }
    setPendingCreate(true); setPendingInvite(""); setPendingRoom(null); setDialog("profile");
  }
  function closeProfile() {
    setDialog(null); setPendingRoom(null); setPendingInvite(""); setPendingCreate(false);
  }
  async function createRoom() {
    setBusy(true);
    try {
      const sc = scenarios.find(item => item.id === scenarioID);
      const secs = duration || durationOptions(sc?.days ?? 300, lang)[1]![1];
      const room = await api.post<Room>("/api/rooms", { scenario_id: scenarioID, day_duration_secs: secs, visibility: "public" });
      setDialog(null); await Promise.all([reloadMine(), reloadPublic()]); navigate(`/rooms/${room.id}`);
    } catch (err) { toast(err instanceof ApiError ? err.message : t("lobby.createFailed")); }
    finally { setBusy(false); }
  }
  function moveEra(event: React.KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (index + 1) % scenarios.length;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp") next = (index - 1 + scenarios.length) % scenarios.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = scenarios.length - 1;
    else return;
    event.preventDefault();
    setScenarioID(scenarios[next]!.id); setDuration(0);
    event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[next]?.focus();
  }

  return <div className="hall">
    <div className="topbar hall-topbar"><div className="brand"><em>●</em> Stocker</div><div className="hall-live"><i /> LIVE HALL</div><div className="spacer"/><DocsLink/><LangSwitch/><div className="avatar">{avatarGlyph(profileUser.avatar_id, profileUser.username)}</div></div>
    <main className="hall-page">
      <section className="hall-hero"><div><span className="hall-eyebrow"><i/> STOCKER / COMMUNITY</span><h1>{c.title}</h1><p>{c.sub}</p></div><div className="hall-actions"><form onSubmit={joinInvite}><input className="hall-invite" aria-label={c.invite} placeholder={c.invite} value={invite} onChange={e=>setInvite(e.target.value)}/><button disabled={!invite.trim()}>{c.join}</button></form><button className="hall-primary" onClick={openCreate}>{c.create}</button></div></section>
      <div className="hall-ribbon"><span><b>{activeRooms}</b>{c.active}</span><span><b>{humanSeats}</b>{c.online}</span><span><b>{scenarios.length}</b>{c.eras}</span>{mostActive?.count ? <span className="ticker">{lang==="zh"?"最活跃":"Most active"} <b>{yearLabel(mostActive.sc)} · {pickL(lang,mostActive.sc.name,mostActive.sc.name_en)}</b></span>:null}</div>
      <div className="hall-grid">
        <section className="hall-panel"><header><div><h2>{c.rooms}</h2><p>{c.roomsSub}</p></div><div className="hall-filters"><button className={`hall-filter ${filter==="all"?"on":""}`} onClick={()=>setFilter("all")}>{c.all}</button>{scenarios.map(sc=><button key={sc.id} className={`hall-filter ${filter===sc.id?"on":""}`} onClick={()=>setFilter(sc.id)}>{yearLabel(sc)}</button>)}</div></header><div className="hall-room-list">{publicRooms.length?publicRooms.map(room=><HallRoomRow key={room.id} room={room} scenarios={scenarios} c={c} lang={lang} onWatch={()=>navigate(`/rooms/${room.id}`)} onJoin={()=>requestJoin(room)}/>):<div className="hall-empty">{c.empty}</div>}</div></section>
        <aside className="hall-panel hall-board"><header><div><h2>{c.board}</h2><p>{c.boardSub}</p></div></header><div className="hall-tabs"><button className={`hall-tab ${boardEra==="all"?"on":""}`} onClick={()=>setBoardEra("all")}>{c.all}</button>{scenarios.map(sc=><button key={sc.id} className={`hall-tab ${boardEra===sc.id?"on":""}`} onClick={()=>setBoardEra(sc.id)}>{yearLabel(sc)}</button>)}</div><div className="hall-leaders">{leaders.length?leaders.map((row,i)=><div className="hall-leader" key={row.scenario_id+row.username}><span className={`hall-rank ${i<3?"top":""}`}>{String(i+1).padStart(2,"0")}</span><span className="hall-avatar">{row.avatar_id?avatarGlyphs[row.avatar_id]:row.username.slice(0,1).toUpperCase()}</span><span className="hall-leader-name"><b>{row.username}</b><small>{yearLabel(scenarios.find(sc=>sc.id===row.scenario_id) ?? {id:row.scenario_id,name:row.scenario_id,days:0})} · {row.wins} {c.wins}</small></span><b className="hall-return">{fmtReturn(row.return_pct)}</b></div>):<div className="hall-empty">—</div>}</div></aside>
      </div>
      {(mine?.rooms?.length??0)>0&&<section className="hall-mine"><header><h2>{c.mine}</h2><span>{mine!.rooms.length}</span></header><div>{mine!.rooms.slice(0,4).map(room=><HallRoomRow key={room.id} room={room} scenarios={scenarios} c={c} lang={lang} dense onWatch={()=>navigate(`/rooms/${room.id}`)}/>)}</div></section>}
    </main>
    {dialog==="profile"&&<div className="hall-dialog-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)closeProfile()}}><form className="hall-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-title" onSubmit={saveProfileAndJoin}><h2 id="profile-title">{c.profileTitle}</h2><p>{c.profileSub}</p><label>{c.displayName}<input autoFocus minLength={2} maxLength={24} required value={displayName} onChange={e=>setDisplayName(e.target.value)}/></label><label>{c.avatar}<div className="hall-avatars">{avatarIDs.map(id=><button type="button" aria-label={id} aria-pressed={avatarID===id} className={`hall-avatar-option ${avatarID===id?"on":""}`} key={id} onClick={()=>setAvatarID(id)}>{avatarGlyphs[id]}</button>)}</div></label><div className="hall-dialog-actions"><button type="button" onClick={closeProfile}>{c.cancel}</button><button className="confirm" disabled={busy||displayName.trim().length<2}>{busy?"…":pendingCreate?c.saveContinue:c.saveJoin}</button></div></form></div>}
    {dialog==="create"&&<div className="hall-dialog-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)setDialog(null)}}><div className="hall-dialog hall-create" role="dialog" aria-modal="true" aria-labelledby="create-title"><h2 id="create-title">{c.createTitle}</h2><p>{c.createSub}</p><div className="era-timeline" role="radiogroup" aria-label={c.era} data-era-count={scenarios.length} style={{"--era-count":Math.max(1,scenarios.length)} as React.CSSProperties}>{scenarios.map((sc,index)=>{const visual=eraVisuals[sc.id];const selected=scenarioID===sc.id;return <button type="button" role="radio" aria-checked={selected} tabIndex={selected?0:-1} data-era-year={scenarioYear(sc)} key={sc.id} className={`era-card ${selected?"selected":""}`} style={{"--era-accent":visual?.accent??"#00c805"} as React.CSSProperties} onClick={()=>{setScenarioID(sc.id);setDuration(0)}} onKeyDown={e=>moveEra(e,index)}>{index<scenarios.length-1&&<span className="era-link"/>}<span className="era-node"/>{visual&&<img className="era-card-art" src={visual.image} alt=""/>}<span className="era-card-shade"/><span className="era-card-index">{String(index+1).padStart(2,"0")}</span><span className="era-card-copy"><strong className="era-year">{yearLabel(sc)}</strong><span className="era-title">{pickL(lang,sc.name,sc.name_en).replace(/^\s*(?:19|20)\d{2}\s*[·—:-]?\s*/,"")}</span><span className="era-days">{sc.days} {lang==="zh"?"个交易日":"trading days"}</span></span><span className="era-card-state">{selected?(lang==="zh"?"已选择":"Selected"):(lang==="zh"?"选择时代":"Select era")}</span></button>})}</div><label>{c.duration}<select aria-label={c.duration} value={duration||durationOptions(scenarios.find(sc=>sc.id===scenarioID)?.days??300,lang)[1]![1]} onChange={e=>setDuration(Number(e.target.value))}>{durationOptions(scenarios.find(sc=>sc.id===scenarioID)?.days??300,lang).map(([label,value])=><option key={value} value={value}>{label}</option>)}</select></label><div className="hall-dialog-actions"><button onClick={()=>setDialog(null)}>{c.cancel}</button><button className="confirm" onClick={createRoom} disabled={busy||!scenarioID}>{busy?c.creating:c.createNow}</button></div></div></div>}
    {node}
  </div>;
}
