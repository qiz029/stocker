import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, AvatarID, EraLeader, PublicRoom, Room, ScenarioInfo, User } from "../api";
import { LangSwitch, pickL, useT } from "../i18n";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUpdateUser, useUser } from "../App";
import DocsLink from "../components/DocsLink";
import MobileNav, { scrollToMobileSection } from "../components/MobileNav";
import nifty1972 from "../assets/eras/nifty-1972.avif";
import crash1987 from "../assets/eras/crash-1987.avif";
import dotcom2000 from "../assets/eras/dotcom-2000.avif";
import gfc2008 from "../assets/eras/gfc-2008.avif";
import "./LobbyHall.css";
import { avatarGlyph, avatarGlyphs, avatarIDs } from "../avatar";
import {
  hallMockEnabled, hallMockLeaders, hallMockMine, hallMockPublicRooms, hallMockScenarios,
} from "../devHallFixtures";

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
type SpeedChoice = { title: string; detail: string; value: number };
function totalTimeLabel(totalSeconds: number, lang: "en" | "zh") {
  const totalMinutes = Math.round(totalSeconds / 60);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (lang === "zh") return `总计约 ${hours} 小时${minutes ? ` ${minutes} 分钟` : ""}`;
  return `~${hours} hr${minutes ? ` ${minutes} min` : ""} total`;
}
function speedOptions(days: number, lang: "en" | "zh"): SpeedChoice[] {
  const pace = (weeks: number) => {
    const value = Math.max(60, Math.round((weeks * 604800) / days));
    const total = lang === "zh" ? `总计 ${weeks} 周` : `${weeks} ${weeks === 1 ? "week" : "weeks"} total`;
    const perDay = lang === "zh" ? `每交易日约 ${Math.round(value / 60)} 分钟` : `~${Math.round(value / 60)} min / trading day`;
    return { value, detail: `${total} · ${perDay}` };
  };
  return [
    { title: lang === "zh" ? "拟真" : "Realistic", ...pace(4) },
    { title: lang === "zh" ? "均衡" : "Balanced", ...pace(2) },
    { title: lang === "zh" ? "快速" : "Fast", ...pace(1) },
    {
      title: lang === "zh" ? "极速" : "Blitz",
      detail: `${totalTimeLabel(days * 60, lang)} · ${lang === "zh" ? "每交易日 1 分钟" : "1 min / trading day"}`,
      value: 60,
    },
  ];
}
function defaultRoomName(user: User, lang: "en" | "zh") {
  const owner = user.display_name?.trim() || (lang === "zh" ? "玩家" : "Player");
  return lang === "zh" ? `${owner}的房间` : `${owner}'s Room`;
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
    roomName: "Room name", era: "Choose an era", duration: "Game speed", createNow: "Create", creating: "Creating room…",
    settings: "Profile",
    account: "Account", accountMenu: "account menu", profileMenu: "Profile", logout: "Log out", logoutFailed: "Log out failed. Please try again.",
    mockNotice: "Mock preview only — no data was changed.", unnamedAlias: "Set alias", aliasTaken: "This alias is already taken.",
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
    roomName: "房间名称", era: "选择时代", duration: "游戏速度", createNow: "创建", creating: "正在创建房间…",
    settings: "账户",
    account: "账户", accountMenu: "账户菜单", profileMenu: "个人资料", logout: "退出登录", logoutFailed: "退出登录失败，请重试。",
    mockNotice: "当前为 Mock 预览，不会修改任何数据。", unnamedAlias: "设置昵称", aliasTaken: "这个昵称已被占用。",
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
        <div className="hall-room-title"><strong>{room.name || `${c.room} #${room.id}`}</strong><span className={`hall-status ${running ? "live" : ""}`}><i />{status}</span></div>
        <div className="hall-room-meta">
          <span>◉ {publicRoom.human_players ?? 1}/{publicRoom.max_human_players ?? 12} {c.players}</span>
          <span className="hall-agent">{c.agent}</span>
          {sc && <span>{pickL(lang, sc.name, sc.name_en)}</span>}
        </div>
        {running && <div className="hall-progress"><i style={{ width: progress + "%" }} /></div>}
      </div>
      <div className="hall-room-move"><b>{fmtReturn(publicRoom.leader_return)}</b><span>{running ? `${lang === "zh" ? "第" : "Day "}${room.current_day ?? 0}/${room.days}${lang === "zh" ? "日" : ""}` : status}</span></div>
      <button className="hall-room-watch" aria-label={`${c.watch} ${room.name || `${c.room} #${room.id}`}`} onClick={onWatch} />
      {!dense && <button className="hall-room-action" onClick={e => { e.stopPropagation(); running || room.ended ? onWatch() : onJoin?.(); }}>{running || room.ended ? c.watch : c.join} →</button>}
    </article>
  );
}

function SpeedPicker({ days, lang, value, label, onChange }: {
  days: number; lang: "en" | "zh"; value: number; label: string; onChange: (value: number) => void;
}) {
  const options = speedOptions(days, lang);
  const active = value || options[1]!.value;
  const [hintedValue, setHintedValue] = useState<number | null>(null);
  function move(event: React.KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (index + 1) % options.length;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp") next = (index - 1 + options.length) % options.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = options.length - 1;
    else return;
    event.preventDefault();
    onChange(options[next]!.value);
    event.currentTarget.closest(".speed-options")?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[next]?.focus();
  }
  return <section className="speed-field">
    <div className="speed-field-head"><span>{label}</span><small>{lang === "zh" ? "悬停查看实际速度" : "Hover to see the exact pace"}</small></div>
    <div className="speed-picker" role="radiogroup" aria-label={label}>
      <div className="speed-options">
        {options.map((option, index) => { const selected = active === option.value; const hinted = hintedValue === option.value; const tipID = `speed-tip-${option.value}`; return <div className="speed-option-wrap" key={option.value}>
          <button type="button" role="radio" aria-label={option.title} aria-checked={selected} aria-describedby={hinted ? tipID : undefined} tabIndex={selected ? 0 : -1}
            className={`speed-option ${selected ? "selected" : ""}`}
            onClick={() => onChange(option.value)} onKeyDown={event => move(event, index)}
            onMouseEnter={() => setHintedValue(option.value)} onMouseLeave={() => setHintedValue(null)}
            onFocus={() => setHintedValue(option.value)} onBlur={() => setHintedValue(null)}>
            <span className="speed-option-dot" aria-hidden="true"/><strong>{option.title}</strong>
          </button>
          {hinted && <span className="speed-tooltip" id={tipID} role="tooltip">{option.detail}</span>}
        </div>; })}
      </div>
      <div className="speed-spectrum" aria-hidden="true">
        <div className="speed-spectrum-track"><div className="speed-spectrum-points">{options.map(option => <i className={active === option.value ? "selected" : ""} key={option.value}/>)}</div></div>
        <div className="speed-spectrum-labels"><span>{lang === "zh" ? "拟真" : "REALISTIC"}</span><span>{lang === "zh" ? "快" : "FAST"}</span></div>
      </div>
    </div>
  </section>;
}

export default function Lobby() {
  const account = useUser();
  const updateAccount = useUpdateUser();
  const { lang, t } = useT();
  const c = hallCopy[lang];
  const navigate = useNavigate();
  const { toast, node } = useToast();
  const mockHall = hallMockEnabled();
  const { data: mine, reload: reloadMine } = usePoll(() => mockHall
    ? Promise.resolve({ rooms: hallMockMine })
    : api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const { data: publicData, reload: reloadPublic } = usePoll(() => mockHall
    ? Promise.resolve({ rooms: hallMockPublicRooms })
    : api.get<{ rooms: PublicRoom[] }>("/api/rooms/public"), 30_000, []);
  const { data: boardData } = usePoll(() => mockHall
    ? Promise.resolve({ items: hallMockLeaders })
    : api.get<{ items: EraLeader[] }>("/api/leaderboards/eras"), 60_000, []);
  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [filter, setFilter] = useState<string>("all");
  const [boardEra, setBoardEra] = useState<string>("all");
  const [invite, setInvite] = useState("");
  const [dialog, setDialog] = useState<"profile" | "create" | null>(null);
  const [pendingRoom, setPendingRoom] = useState<PublicRoom | null>(null);
  const [pendingInvite, setPendingInvite] = useState("");
  const [pendingCreate, setPendingCreate] = useState(false);
  const [profileUser, setProfileUser] = useState<User>(account);
  const accountAlias = profileUser.display_name?.trim() || c.unnamedAlias;
  const [displayName, setDisplayName] = useState(account.display_name ?? "");
  const [avatarID, setAvatarID] = useState<AvatarID>(account.avatar_id ?? "bull");
  const [scenarioID, setScenarioID] = useState("");
  const [roomName, setRoomName] = useState("");
  const [duration, setDuration] = useState(0);
  const [busy, setBusy] = useState(false);
  const [mobileTab, setMobileTab] = useState("market");
  const [accountOpen, setAccountOpen] = useState(false);
  const accountRef = useRef<HTMLDivElement>(null);
  const accountButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => { (mockHall
    ? Promise.resolve({ items: hallMockScenarios })
    : api.get<{ items: ScenarioInfo[] }>("/api/scenarios")).then(res => {
    const items = chronologicalScenarios(res.items ?? []); setScenarios(items); if (items[0]) setScenarioID(current => current || items[0]!.id);
  }).catch(() => setScenarios([])); }, [mockHall]);
  useEffect(() => {
    if (!dialog) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (dialog === "profile") closeProfile();
      else setDialog(null);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [dialog]);
  useEffect(() => {
    if (!accountOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!accountRef.current?.contains(event.target as Node)) setAccountOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setAccountOpen(false);
      accountButtonRef.current?.focus();
    };
    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [accountOpen]);

  const publicRooms = (publicData?.rooms ?? []).filter(room => filter === "all" || room.scenario_id === filter);
  const leaders = (boardData?.items ?? []).filter(row => boardEra === "all" || row.scenario_id === boardEra).slice(0, 8);
  const activeRooms = (publicData?.rooms ?? []).filter(room => room.status === "running" && !room.ended).length;
  const humanSeats = (publicData?.rooms ?? []).reduce((sum, room) => sum + (room.human_players ?? 1), 0);
  const mostActive = useMemo(() => scenarios.map(sc => ({ sc, count: (publicData?.rooms ?? []).filter(r => r.scenario_id === sc.id).length })).sort((a,b) => b.count-a.count)[0], [publicData, scenarios]);

  async function joinInvite(e: FormEvent) {
    e.preventDefault();
    const code = invite.trim();
    if (mockHall) { setInvite(""); toast(c.mockNotice); return; }
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
    if (mockHall) { toast(c.mockNotice); return; }
    if (profileUser.profile_complete) { void joinPublic(room); return; }
    setPendingCreate(false); setPendingRoom(room); setDialog("profile");
  }
  async function joinPublic(room: PublicRoom) {
    if (mockHall) { toast(c.mockNotice); return; }
    setBusy(true);
    try { await api.post(`/api/rooms/${room.id}/join`); await Promise.all([reloadMine(), reloadPublic()]); navigate(`/rooms/${room.id}`); }
    catch (err) { toast(err instanceof ApiError ? err.message : t("lobby.joinFailed")); }
    finally { setBusy(false); }
  }
  async function saveProfileAndJoin(e: FormEvent) {
    e.preventDefault(); setBusy(true);
    try {
      const updated = mockHall
        ? { ...profileUser, display_name: displayName.trim(), avatar_id: avatarID, profile_complete: true }
        : await api.put<User>("/api/me/profile", { display_name: displayName.trim(), avatar_id: avatarID });
      setProfileUser(updated); updateAccount(updated);
      const targetRoom = pendingRoom; const targetInvite = pendingInvite; const shouldCreate = pendingCreate;
      setPendingRoom(null); setPendingInvite(""); setPendingCreate(false);
      if (mockHall) { setDialog(null); toast(c.mockNotice); }
      else if (targetRoom) await joinPublic(targetRoom);
      else if (targetInvite) {
        const room = await api.post<Room>("/api/rooms/join", { invite_code: targetInvite });
        navigate(`/rooms/${room.id}`);
      } else if (shouldCreate) {
        setRoomName(defaultRoomName(updated, lang));
        setDialog("create");
      } else setDialog(null);
    } catch (err) { toast(err instanceof ApiError && err.message === "alias already in use" ? c.aliasTaken : err instanceof ApiError ? err.message : t("lobby.joinFailed")); }
    finally { setBusy(false); }
  }
  function openCreate() {
    const hasIdentity = profileUser.profile_complete
      || Boolean(profileUser.display_name?.trim() && profileUser.avatar_id);
    if (!hasIdentity) {
      setPendingCreate(true); setPendingInvite(""); setPendingRoom(null); setDialog("profile"); return;
    }
    setRoomName(defaultRoomName(profileUser, lang));
    setDialog("create");
  }
  function closeProfile() {
    setDialog(null); setPendingRoom(null); setPendingInvite(""); setPendingCreate(false);
  }
  function openSettings() {
    setAccountOpen(false);
    navigate(mockHall ? "/profile?mock=hall" : "/profile");
  }
  async function logout() {
    setAccountOpen(false);
    if (!mockHall) {
      try { await api.post("/api/logout"); }
      catch (err) {
        if (!(err instanceof ApiError && err.status === 401)) {
          toast(err instanceof ApiError ? err.message : c.logoutFailed);
          return;
        }
      }
    }
    updateAccount(null);
    navigate("/");
  }
  function moveAccountMenu(event: React.KeyboardEvent<HTMLDivElement>) {
    const items = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')];
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    let next = current;
    if (event.key === "ArrowDown") next = (current + 1) % items.length;
    else if (event.key === "ArrowUp") next = (current - 1 + items.length) % items.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = items.length - 1;
    else return;
    event.preventDefault();
    items[next]?.focus();
  }
  async function createRoom() {
    if (mockHall) { setDialog(null); toast(c.mockNotice); return; }
    setBusy(true);
    try {
      const sc = scenarios.find(item => item.id === scenarioID);
      const secs = duration || speedOptions(sc?.days ?? 300, lang)[1]!.value;
      const room = await api.post<Room>("/api/rooms", { name: roomName.trim(), scenario_id: scenarioID, day_duration_secs: secs, visibility: "public" });
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

  function selectMobileTab(tab: string, section?: string) {
    setMobileTab(tab);
    if (section) scrollToMobileSection(section);
  }

  function openRoom(roomID: number) {
    if (mockHall) { toast(c.mockNotice); return; }
    navigate(`/rooms/${roomID}`);
  }

  return <div className="hall has-mobile-nav">
    <div className="topbar hall-topbar"><div className="brand"><em>●</em> Stocker</div><div className="hall-live"><i /> LIVE HALL</div><div className="spacer"/><DocsLink/><LangSwitch/><div className="hall-account" ref={accountRef}><button ref={accountButtonRef} type="button" className="hall-account-trigger" aria-label={`${accountAlias} ${c.accountMenu}`} aria-haspopup="menu" aria-expanded={accountOpen} onClick={()=>setAccountOpen(open=>!open)} onKeyDown={event=>{if(event.key!=="ArrowDown"&&event.key!=="ArrowUp")return;event.preventDefault();setAccountOpen(true);const last=event.key==="ArrowUp";requestAnimationFrame(()=>{const items=accountRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]');items?.[last?items.length-1:0]?.focus()})}}><span className="avatar">{avatarGlyph(profileUser.avatar_id, accountAlias)}</span><span className="hall-account-name">{accountAlias}</span><span className="hall-account-chevron" aria-hidden="true">⌄</span></button>{accountOpen&&<div className="hall-account-menu" role="menu" aria-label={c.account} onKeyDown={moveAccountMenu}><button type="button" role="menuitem" onClick={openSettings}><span aria-hidden="true">◎</span>{c.profileMenu}</button><button type="button" role="menuitem" className="logout" onClick={()=>void logout()}><span aria-hidden="true">↗</span>{c.logout}</button></div>}</div></div>
    <main className="hall-page">
      <section className="hall-hero"><div><span className="hall-eyebrow"><i/> STOCKER / COMMUNITY</span><h1>{c.title}</h1><p>{c.sub}</p></div><div className="hall-actions"><form onSubmit={joinInvite}><input className="hall-invite" aria-label={c.invite} placeholder={c.invite} value={invite} onChange={e=>setInvite(e.target.value)}/><button disabled={!invite.trim()}>{c.join}</button></form><button className="hall-primary" onClick={openCreate}>{c.create}</button></div></section>
      <div className="hall-ribbon"><span><b>{activeRooms}</b>{c.active}</span><span><b>{humanSeats}</b>{c.online}</span><span><b>{scenarios.length}</b>{c.eras}</span>{mostActive?.count ? <span className="ticker">{lang==="zh"?"最活跃":"Most active"} <b>{yearLabel(mostActive.sc)} · {pickL(lang,mostActive.sc.name,mostActive.sc.name_en)}</b></span>:null}</div>
      <div className="hall-grid">
        <section className="hall-panel" id="mobile-market"><header><div><h2>{c.rooms}</h2><p>{c.roomsSub}</p></div><div className="hall-filters"><button className={`hall-filter ${filter==="all"?"on":""}`} onClick={()=>setFilter("all")}>{c.all}</button>{scenarios.map(sc=><button key={sc.id} className={`hall-filter ${filter===sc.id?"on":""}`} onClick={()=>setFilter(sc.id)}>{yearLabel(sc)}</button>)}</div></header><div className="hall-room-list">{publicRooms.length?publicRooms.map(room=><HallRoomRow key={room.id} room={room} scenarios={scenarios} c={c} lang={lang} onWatch={()=>openRoom(room.id)} onJoin={()=>requestJoin(room)}/>):<div className="hall-empty">{c.empty}</div>}</div></section>
        <aside className="hall-panel hall-board" id="mobile-rankings"><header><div><h2>{c.board}</h2><p>{c.boardSub}</p></div></header><div className="hall-tabs"><button className={`hall-tab ${boardEra==="all"?"on":""}`} onClick={()=>setBoardEra("all")}>{c.all}</button>{scenarios.map(sc=><button key={sc.id} className={`hall-tab ${boardEra===sc.id?"on":""}`} onClick={()=>setBoardEra(sc.id)}>{yearLabel(sc)}</button>)}</div><div className="hall-leaders">{leaders.length?leaders.map((row,i)=><div className="hall-leader" key={row.scenario_id+row.username}><span className={`hall-rank ${i<3?"top":""}`}>{String(i+1).padStart(2,"0")}</span><span className="hall-avatar">{row.avatar_id?avatarGlyphs[row.avatar_id]:row.username.slice(0,1).toUpperCase()}</span><span className="hall-leader-name"><b>{row.username}</b><small>{yearLabel(scenarios.find(sc=>sc.id===row.scenario_id) ?? {id:row.scenario_id,name:row.scenario_id,days:0})} · {row.wins} {c.wins}</small></span><b className="hall-return">{fmtReturn(row.return_pct)}</b></div>):<div className="hall-empty">—</div>}</div></aside>
      </div>
      {(mine?.rooms?.length??0)>0&&<section className="hall-mine" id="mobile-my-rooms"><header><h2>{c.mine}</h2><span>{mine!.rooms.length}</span></header><div>{mine!.rooms.slice(0,4).map(room=><HallRoomRow key={room.id} room={room} scenarios={scenarios} c={c} lang={lang} dense onWatch={()=>openRoom(room.id)}/>)}</div></section>}
    </main>
    {dialog==="profile"&&<div className="hall-dialog-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)closeProfile()}}><form className="hall-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-title" onSubmit={saveProfileAndJoin}><h2 id="profile-title">{c.profileTitle}</h2><p>{c.profileSub}</p><label>{c.displayName}<input autoFocus minLength={2} maxLength={24} required value={displayName} onChange={e=>setDisplayName(e.target.value)}/></label><label>{c.avatar}<div className="hall-avatars">{avatarIDs.map(id=><button type="button" aria-label={id} aria-pressed={avatarID===id} className={`hall-avatar-option ${avatarID===id?"on":""}`} key={id} onClick={()=>setAvatarID(id)}>{avatarGlyphs[id]}</button>)}</div></label><div className="hall-dialog-actions"><button type="button" onClick={closeProfile}>{c.cancel}</button><button className="confirm" disabled={busy||displayName.trim().length<2}>{busy?"…":pendingCreate?c.saveContinue:c.saveJoin}</button></div></form></div>}
    {dialog==="create"&&<div className="hall-dialog-backdrop" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)setDialog(null)}}><div className="hall-dialog hall-create" role="dialog" aria-modal="true" aria-labelledby="create-title"><h2 id="create-title">{c.createTitle}</h2><p>{c.createSub}</p><label>{c.roomName}<input autoFocus aria-label={c.roomName} minLength={2} maxLength={40} required value={roomName} onChange={e=>setRoomName(e.target.value)}/></label><div className="era-timeline" role="radiogroup" aria-label={c.era} data-era-count={scenarios.length} style={{"--era-count":Math.max(1,scenarios.length)} as React.CSSProperties}>{scenarios.map((sc,index)=>{const visual=eraVisuals[sc.id];const selected=scenarioID===sc.id;return <button type="button" role="radio" aria-checked={selected} tabIndex={selected?0:-1} data-era-year={scenarioYear(sc)} key={sc.id} className={`era-card ${selected?"selected":""}`} style={{"--era-accent":visual?.accent??"#00c805"} as React.CSSProperties} onClick={()=>{setScenarioID(sc.id);setDuration(0)}} onKeyDown={e=>moveEra(e,index)}>{index<scenarios.length-1&&<span className="era-link"/>}<span className="era-node"/>{visual&&<img className="era-card-art" src={visual.image} alt=""/>}<span className="era-card-shade"/><span className="era-card-index">{String(index+1).padStart(2,"0")}</span><span className="era-card-copy"><strong className="era-year">{yearLabel(sc)}</strong><span className="era-title">{pickL(lang,sc.name,sc.name_en).replace(/^\s*(?:19|20)\d{2}\s*[·—:-]?\s*/,"")}</span><span className="era-days">{sc.days} {lang==="zh"?"个交易日":"trading days"}</span></span><span className="era-card-state">{selected?(lang==="zh"?"已选择":"Selected"):(lang==="zh"?"选择时代":"Select era")}</span></button>})}</div><SpeedPicker days={scenarios.find(sc=>sc.id===scenarioID)?.days??300} lang={lang} value={duration} label={c.duration} onChange={setDuration}/><div className="hall-dialog-actions"><button onClick={()=>setDialog(null)}>{c.cancel}</button><button className="confirm" onClick={createRoom} disabled={busy||!scenarioID||roomName.trim().length<2}>{busy?c.creating:c.createNow}</button></div></div></div>}
    <MobileNav
      label={lang === "zh" ? "大厅导航" : "Hall navigation"}
      active={mobileTab}
      items={[
        { id: "market", icon: "⌁", label: lang === "zh" ? "市场" : "Market", onSelect: () => selectMobileTab("market", "mobile-market") },
        { id: "rankings", icon: "↗", label: lang === "zh" ? "排行" : "Ranks", onSelect: () => selectMobileTab("rankings", "mobile-rankings") },
        { id: "create", icon: "+", label: lang === "zh" ? "开房" : "Create", ariaLabel: lang === "zh" ? "打开创建房间" : "Open create room", primary: true, onSelect: () => { setMobileTab("create"); openCreate(); } },
        { id: "mine", icon: "◎", label: lang === "zh" ? "我的" : "Mine", onSelect: () => { setMobileTab("mine"); (mine?.rooms?.length ?? 0) > 0 ? scrollToMobileSection("mobile-my-rooms") : openCreate(); } },
        { id: "settings", icon: avatarGlyph(profileUser.avatar_id, accountAlias), label: c.settings, onSelect: openSettings },
      ]}
    />
    {node}
  </div>;
}
