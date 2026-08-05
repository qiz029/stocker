import React, { useEffect, useState } from "react";
import {
  RefreshControl, ScrollView, StyleSheet, Text, TextInput, TouchableOpacity, View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useRouter } from "expo-router";
import { api, ApiError, EraLeader, PublicRoom, Room, ScenarioInfo } from "@core/api";
import { pickL } from "@core/i18n";
import { usePoll } from "@core/usePoll";
import { useSession } from "../src/session";
import LangToggle from "../src/components/LangToggle";
import Avatar from "../src/components/Avatar";
import { chronologicalScenarios, durationOptions, fmtReturn, yearLabel } from "../src/era";
import { colors } from "../src/theme";

function statusOf(room: Room): "lobby" | "running" | "ended" {
  if (room.status === "lobby") return "lobby";
  return room.ended ? "ended" : "running";
}

export default function HallScreen() {
  const { t, lang, user, logout } = useSession();
  const router = useRouter();
  const { data: mine, error, reload: reloadMine } = usePoll(
    () => api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const { data: publicData, reload: reloadPublic } = usePoll(
    () => api.get<{ rooms: PublicRoom[] }>("/api/rooms/public"), 30_000, []);
  const { data: boardData } = usePoll(
    () => api.get<{ items: EraLeader[] }>("/api/leaderboards/eras"), 60_000, []);

  const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);
  const [code, setCode] = useState("");
  const [joinError, setJoinError] = useState<string | null>(null);
  const [joining, setJoining] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  // create-room form
  const [scenarioID, setScenarioID] = useState("");
  const [duration, setDuration] = useState(0);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  useEffect(() => {
    api.get<{ items: ScenarioInfo[] }>("/api/scenarios")
      .then(res => {
        const sorted = chronologicalScenarios(res.items);
        setScenarios(sorted);
        if (sorted.length > 0) setScenarioID(id => id || sorted[0]!.id);
      })
      .catch(() => undefined);
  }, []);

  const scenarioName = (id: string) => {
    const sc = scenarios.find(s => s.id === id);
    return sc ? pickL(lang, sc.name, sc.name_en) : t("era.name");
  };
  const selected = scenarios.find(s => s.id === scenarioID);
  const durations = durationOptions(selected?.days ?? 300, lang);
  const activeDuration = duration || durations[1]?.[1] || 60;

  async function joinByCode() {
    const invite = code.trim();
    if (!invite) return;
    setJoining(true);
    setJoinError(null);
    try {
      const room = await api.post<Room>("/api/rooms/join", { invite_code: invite });
      setCode("");
      router.push(`/room/${room.id}`);
    } catch (e) {
      setJoinError(e instanceof ApiError ? e.message : t("lobby.joinFailed"));
    } finally {
      setJoining(false);
    }
  }

  async function joinPublic(room: PublicRoom) {
    try {
      await api.post(`/api/rooms/${room.id}/join`);
      router.push(`/room/${room.id}`);
    } catch (e) {
      setJoinError(e instanceof ApiError ? e.message : t("lobby.joinFailed"));
    }
  }

  async function createRoom() {
    if (!scenarioID) return;
    setCreating(true);
    setCreateError(null);
    try {
      const room = await api.post<Room>("/api/rooms", {
        scenario_id: scenarioID, day_duration_secs: activeDuration, visibility: "public",
      });
      router.push(`/room/${room.id}`);
    } catch (e) {
      setCreateError(e instanceof ApiError ? e.message : t("lobby.createFailed"));
    } finally {
      setCreating(false);
    }
  }

  async function onRefresh() {
    setRefreshing(true);
    await Promise.all([reloadMine(), reloadPublic()]);
    setRefreshing(false);
  }

  const roomCard = (room: Room, actions?: React.ReactNode) => {
    const st = statusOf(room);
    const day = room.current_day ?? 0;
    const progress = room.days > 0 ? Math.min(1, day / room.days) : 0;
    return (
      <TouchableOpacity key={room.id} style={styles.card} onPress={() => router.push(`/room/${room.id}`)}>
        <View style={styles.cardTop}>
          <Text style={styles.cardTitle}>{scenarioName(room.scenario_id)}</Text>
          <Text style={[styles.tag, st === "running" ? styles.tagLive : styles.tagDone]}>
            {st === "lobby" ? t("status.waiting") : st === "ended" ? t("status.ended") : t("status.running")}
          </Text>
        </View>
        <Text style={styles.meta}>
          {t("lobby.dayA")} {day} {t("lobby.dayB", { days: room.days })}
          {room.invite_code ? `  ·  ${t("lobby.inviteA")} ${room.invite_code}` : ""}
        </Text>
        <View style={styles.progress}>
          <View style={[styles.progressFill, { flex: progress }]} />
          <View style={{ flex: 1 - progress }} />
        </View>
        {actions}
      </TouchableOpacity>
    );
  };

  const publicRooms = publicData?.rooms ?? [];
  const leaders = (boardData?.items ?? []).slice(0, 8);

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <Text style={styles.brand}><Text style={{ color: colors.up }}>●</Text> Stocker</Text>
        <View style={styles.topActions}>
          <LangToggle />
          {user && <Avatar id={user.avatar_id} username={user.username} size={28} />}
          <TouchableOpacity onPress={() => void logout()} hitSlop={8}>
            <Text style={styles.logout}>⏻</Text>
          </TouchableOpacity>
        </View>
      </View>

      <ScrollView contentContainerStyle={styles.list}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.ink2} />}>
        <Text style={styles.h1}>{t("lobby.title")}</Text>
        <Text style={styles.sub}>{t("lobby.sub")}</Text>

        <View style={styles.joinRow}>
          <TextInput style={styles.joinInput} placeholder={t("lobby.joinPlaceholder")}
            placeholderTextColor={colors.ink3} autoCapitalize="characters" autoCorrect={false}
            value={code} onChangeText={setCode} onSubmitEditing={joinByCode} />
          <TouchableOpacity style={[styles.joinBtn, (!code.trim() || joining) && styles.disabled]}
            disabled={!code.trim() || joining} onPress={joinByCode}>
            <Text style={styles.joinBtnTxt}>{t("lobby.join")}</Text>
          </TouchableOpacity>
        </View>
        {joinError && <Text style={styles.error}>{joinError}</Text>}
        {error && <Text style={styles.error}>{error}</Text>}

        {/* create room */}
        <View style={styles.card}>
          <Text style={styles.sectionTitle}>{t("lobby.newRoom")}</Text>
          <Text style={styles.fieldLabel}>{t("lobby.chooseEra")}</Text>
          {scenarios.map(sc => (
            <TouchableOpacity key={sc.id} style={[styles.eraRow, scenarioID === sc.id && styles.eraRowOn]}
              onPress={() => { setScenarioID(sc.id); setDuration(0); }}>
              <Text style={styles.eraYear}>{yearLabel(sc)}</Text>
              <Text style={styles.eraName}>{pickL(lang, sc.name, sc.name_en)}</Text>
              <Text style={styles.meta}>{t("lobby.tradingDays", { days: sc.days })}</Text>
            </TouchableOpacity>
          ))}
          <Text style={[styles.fieldLabel, { marginTop: 10 }]}>{t("lobby.gameDuration")}</Text>
          <View style={styles.durations}>
            {durations.map(([label, secs]) => (
              <TouchableOpacity key={secs} style={[styles.chip, activeDuration === secs && styles.chipOn]}
                onPress={() => setDuration(secs)}>
                <Text style={[styles.chipTxt, activeDuration === secs && styles.chipTxtOn]}>{label}</Text>
              </TouchableOpacity>
            ))}
          </View>
          {createError && <Text style={styles.error}>{createError}</Text>}
          <TouchableOpacity style={[styles.submit, (creating || !scenarioID) && styles.disabled]}
            disabled={creating || !scenarioID} onPress={createRoom}>
            <Text style={styles.submitTxt}>{creating ? t("lobby.creating") : t("lobby.create")}</Text>
          </TouchableOpacity>
        </View>

        {/* my rooms */}
        <Text style={styles.sectionTitle}>{t("lobby.title")}</Text>
        {(mine?.rooms ?? []).map(r => roomCard(r))}

        {/* public rooms */}
        <Text style={styles.sectionTitle}>{t("hall.publicRooms")}</Text>
        {publicRooms.length === 0 && <Text style={styles.meta}>{t("hall.empty")}</Text>}
        {publicRooms.map(r => roomCard(r, (
          <View style={styles.pubActions}>
            <Text style={styles.meta}>
              {r.human_players}/{r.max_human_players} · {r.agent_players} × {t("common.agent")}
              {r.leader_name ? `  ·  ${r.leader_name} ${fmtReturn(r.leader_return)}` : ""}
            </Text>
            {r.is_member === false && (
              <TouchableOpacity onPress={() => void joinPublic(r)} hitSlop={6}>
                <Text style={styles.joinLink}>{t("lobby.join")}</Text>
              </TouchableOpacity>
            )}
          </View>
        )))}

        {/* era leaderboard */}
        <Text style={styles.sectionTitle}>{t("hall.board")}</Text>
        <View style={styles.card}>
          {leaders.length === 0 && <Text style={styles.meta}>—</Text>}
          {leaders.map((row, i) => (
            <View key={row.scenario_id + row.username} style={styles.leaderRow}>
              <Text style={styles.rank}>{String(i + 1).padStart(2, "0")}</Text>
              <Avatar id={row.avatar_id} username={row.username} size={24} />
              <View style={{ flex: 1 }}>
                <Text style={styles.leaderName}>{row.username}</Text>
                <Text style={styles.meta}>
                  {scenarioName(row.scenario_id)} · {row.wins} {t("hall.wins")}
                </Text>
              </View>
              <Text style={[styles.leaderRet, (row.return_pct ?? 0) >= 0 ? styles.up : styles.down]}>
                {fmtReturn(row.return_pct)}
              </Text>
            </View>
          ))}
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.bg },
  topbar: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: 20, paddingVertical: 12,
    borderBottomWidth: 1, borderBottomColor: colors.line,
  },
  brand: { color: colors.ink, fontSize: 17, fontWeight: "700" },
  topActions: { flexDirection: "row", alignItems: "center", gap: 14 },
  logout: { color: colors.ink2, fontSize: 18 },
  list: { padding: 20, paddingBottom: 48 },
  h1: { color: colors.ink, fontSize: 24, fontWeight: "700", marginTop: 8 },
  sub: { color: colors.ink2, fontSize: 14, marginTop: 2, marginBottom: 18 },
  joinRow: { flexDirection: "row", gap: 8, marginBottom: 10 },
  joinInput: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 10, color: colors.ink, fontSize: 14,
  },
  joinBtn: { backgroundColor: colors.upSoft, borderRadius: 10, paddingHorizontal: 18, justifyContent: "center" },
  joinBtnTxt: { color: colors.up, fontWeight: "700", fontSize: 14 },
  disabled: { opacity: 0.35 },
  error: { color: colors.down, fontSize: 13, marginBottom: 10 },
  sectionTitle: {
    color: colors.ink2, fontSize: 12, fontWeight: "700", letterSpacing: 1.2,
    textTransform: "uppercase", marginTop: 18, marginBottom: 8,
  },
  fieldLabel: { color: colors.ink2, fontSize: 12, marginBottom: 6, marginTop: 4 },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 14, padding: 18, marginBottom: 12,
  },
  eraRow: {
    flexDirection: "row", alignItems: "center", gap: 10,
    borderWidth: 1, borderColor: colors.line, borderRadius: 10,
    paddingHorizontal: 12, paddingVertical: 10, marginBottom: 6,
  },
  eraRowOn: { borderColor: colors.up },
  eraYear: { color: colors.ink, fontSize: 15, fontWeight: "700", fontVariant: ["tabular-nums"] },
  eraName: { color: colors.ink2, fontSize: 13, flex: 1 },
  durations: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
  chip: {
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6,
  },
  chipOn: { backgroundColor: colors.up, borderColor: colors.up },
  chipTxt: { color: colors.ink2, fontSize: 12 },
  chipTxtOn: { color: "#04140a", fontWeight: "600" },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 12,
    alignItems: "center", marginTop: 14,
  },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  cardTop: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 10 },
  cardTitle: { color: colors.ink, fontSize: 16, fontWeight: "600", flex: 1 },
  tag: { fontSize: 11, paddingHorizontal: 10, paddingVertical: 2, borderRadius: 999, overflow: "hidden" },
  tagLive: { color: colors.up, backgroundColor: colors.upSoft },
  tagDone: { color: colors.ink2, backgroundColor: colors.card2 },
  meta: { color: colors.ink2, fontSize: 12, marginTop: 4, fontVariant: ["tabular-nums"] },
  progress: { flexDirection: "row", height: 4, backgroundColor: colors.card2, borderRadius: 4, marginTop: 12, overflow: "hidden" },
  progressFill: { backgroundColor: colors.up, borderRadius: 4 },
  pubActions: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    marginTop: 10, gap: 10,
  },
  joinLink: { color: colors.up, fontSize: 13, fontWeight: "700" },
  leaderRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 7 },
  rank: { color: colors.ink3, fontSize: 12, width: 20, fontVariant: ["tabular-nums"] },
  leaderName: { color: colors.ink, fontSize: 14, fontWeight: "600" },
  leaderRet: { fontSize: 14, fontWeight: "600", fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
});
