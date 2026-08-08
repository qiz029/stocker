import type { EraLeader, PublicRoom, Room, ScenarioInfo, User } from "./api";

export function hallMockEnabled() {
  return import.meta.env.DEV
    && typeof window !== "undefined"
    && new URLSearchParams(window.location.search).get("mock") === "hall";
}

export const hallMockUser: User = {
  id: 9000,
  username: "market_owl",
  display_name: "市场猫头鹰",
  avatar_id: "owl",
  email: "market.owl@example.com",
  description: "长期价值投资者，也喜欢在市场最嘈杂的时候保持耐心。关注科技周期、宏观叙事和群体心理。",
  social_links: {
    website: "https://example.com/market-owl",
    x: "https://x.com/market_owl",
    github: "https://github.com/market-owl",
    linkedin: "https://www.linkedin.com/in/market-owl",
  },
  profile_complete: true,
};

export const hallMockScenarios: ScenarioInfo[] = [
  { id: "nifty-1972", name: "1972 漂亮50与石油危机", name_en: "1972 Nifty Fifty & Oil Crisis", days: 881 },
  { id: "crash-1987", name: "1987 黑色星期一", name_en: "1987 Black Monday", days: 759 },
  { id: "dotcom-2000", name: "2000 互联网泡沫", name_en: "2000 Dot-com Bubble", days: 752 },
  { id: "gfc-2008", name: "2008 金融危机", name_en: "2008 Global Financial Crisis", days: 819 },
];

export const hallMockPublicRooms: PublicRoom[] = [
  {
    id: 9101, name: "黑色星期一抄底局", scenario_id: "crash-1987", days: 759,
    status: "running", day_duration_secs: 900, current_day: 88, ended: false,
    visibility: "public", is_member: false, human_players: 7, max_human_players: 12,
    agent_players: 5, leader_name: "逆风交易员", leader_avatar: "fox", leader_return: 0.184,
  },
  {
    id: 9102, name: "互联网泡沫最后一舞", scenario_id: "dotcom-2000", days: 752,
    status: "lobby", day_duration_secs: 1200, current_day: 0, ended: false,
    visibility: "public", is_member: false, human_players: 4, max_human_players: 12,
    agent_players: 5,
  },
  {
    id: 9103, name: "次贷危机幸存者联盟", scenario_id: "gfc-2008", days: 819,
    status: "running", day_duration_secs: 600, current_day: 436, ended: false,
    visibility: "public", is_member: true, human_players: 11, max_human_players: 12,
    agent_players: 5, leader_name: "现金为王", leader_avatar: "bear", leader_return: -0.037,
  },
  {
    id: 9104, name: "漂亮50长期价值投资研究小组", scenario_id: "nifty-1972", days: 881,
    status: "running", day_duration_secs: 1800, current_day: 802, ended: true,
    visibility: "public", is_member: false, human_players: 12, max_human_players: 12,
    agent_players: 5, leader_name: "复利火箭", leader_avatar: "rocket", leader_return: 1.276,
  },
  {
    id: 9105, name: "Friday Night Traders", scenario_id: "dotcom-2000", days: 752,
    status: "running", day_duration_secs: 300, current_day: 21, ended: false,
    visibility: "public", is_member: false, human_players: 2, max_human_players: 12,
    agent_players: 5, leader_name: "Diamond Hands", leader_avatar: "diamond", leader_return: 0.052,
  },
];

export const hallMockMine: Room[] = [
  { ...hallMockPublicRooms[2]!, invite_code: "M0CK88", is_host: false, is_member: true },
  {
    id: 9201, name: "市场猫头鹰的房间", invite_code: "OWL200", scenario_id: "dotcom-2000",
    days: 752, status: "lobby", day_duration_secs: 1200, current_day: 0, ended: false,
    visibility: "public", is_host: true, is_member: true,
  },
  {
    id: 9202, name: "石油危机观察站", invite_code: "OIL72", scenario_id: "nifty-1972",
    days: 881, status: "running", day_duration_secs: 900, current_day: 317, ended: false,
    visibility: "private", is_host: true, is_member: true,
  },
];

export const hallMockLeaders: EraLeader[] = [
  { scenario_id: "nifty-1972", username: "复利火箭", avatar_id: "rocket", return_pct: 1.276, wins: 4 },
  { scenario_id: "dotcom-2000", username: "泡沫冲浪者", avatar_id: "shark", return_pct: 0.864, wins: 3 },
  { scenario_id: "gfc-2008", username: "现金为王", avatar_id: "bear", return_pct: 0.492, wins: 2 },
  { scenario_id: "crash-1987", username: "逆风交易员", avatar_id: "fox", return_pct: 0.311, wins: 2 },
  { scenario_id: "dotcom-2000", username: "Diamond Hands", avatar_id: "diamond", return_pct: 0.184, wins: 1 },
  { scenario_id: "gfc-2008", username: "抄底老虎", avatar_id: "tiger", return_pct: 0.097, wins: 1 },
];
