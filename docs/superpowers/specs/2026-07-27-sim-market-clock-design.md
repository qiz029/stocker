# 模拟市场时钟(Sim Market Clock)设计

日期:2026-07-27
状态:已与需求方确认方向(纯展示节奏 + 虚构日历)

## 背景与目标

当前房间时间推进是纯线性的:每过 `day_duration_secs` 墙钟时间前进一个历史交易日,
UI 只显示"第 N 个交易日"和"距下一交易日 MM:SS"倒计时,体感机械。

目标:让时间流逝有市场质感——盘中时钟按 2-4 小时一跳推进,有收盘时段,
周五收盘后进入"周末休市",下个交易日跳到下周一。

约束:

- **服务器零改动**。交易日推进节奏、结算机制(当日下单、次日开盘成交)全部不变。
- **纯展示层**:收盘/周末不增加玩家真实等待时间。
- **不剧透场景**:场景是历史盲盒(1987/1999/2008…),不能用真实历史日期,
  使用虚构日历("第 W 周 · 周X")。

## 方案

### 核心模块:`web/src/simClock.ts`(纯函数)

把已有数据 `(started_at, day_duration_secs, roomId, days, 当前墙钟)` 映射为时钟状态:

```ts
export type ClockPhase = "open" | "closed" | "weekend" | "ended";

export interface SimClockState {
  day: number;          // 交易日索引(与服务器 current_day 一致的本地推算)
  week: number;         // 1 起始:floor(day / 5) + 1
  weekday: number;      // 0..4 → 周一..周五:day % 5
  dateLabel: string;    // "第3周 · 周四"
  time: string | null;  // 盘中为 "14:10",其余为 null
  phase: ClockPhase;
  nextOpenSecs: number | null; // closed/weekend 时距下一开盘的秒数,其余为 null
}

export function simClock(
  startedAt: string,
  dayDurationSecs: number,
  roomId: number,
  totalDays: number,
  nowMs?: number,       // 默认 Date.now(),测试时注入
): SimClockState;

export function dayLabel(day: number): string; // "第3周 · 周四",供图表悬停复用
```

### 每个交易日的墙钟区间切分

一个交易日占墙钟 `D = day_duration_secs` 秒,切成两段:

- **交易时段(前 78%)**:模拟时钟从 9:30 走到 16:00(390 分钟),
  分成 2~3 跳,每跳间隔 120~240 分钟(即 2-4 小时),间隔取 5 分钟的整数倍,
  末点恰为 16:00。共 n 跳则有 n+1 个显示时刻(含 9:30 与 16:00);
  交易时段墙钟均分为 n+1 片,第 i 片显示第 i 个时刻,跨片边界时时钟"跳动"。
- **收盘时段(后 22%)**:`phase = "closed"`,显示"已收盘"及距下一开盘的倒计时。
  当天为周五(`day % 5 == 4`)时 `phase = "weekend"`,显示"周末休市";
  下一交易日的日历自然落到下周一(weekday 数学自动成立)。

跳点由确定性 PRNG 生成,种子为 `hash(roomId, day)`(如 mulberry32 变体):
同房间所有玩家、任意时刻刷新,看到完全相同的时钟序列。

跳数与间隔的可行域:n=2 时两段间隔各在 [150, 240] 分钟;
n=3 时三段各在 [120, 240] 且总和 390(即前两段之和落在 [240, 270])。
实现用构造式采样(先定 n,再在可行区间内均匀采样),不用拒绝采样。

### 边界情形

- 房间未启动(`started_at == null`):不渲染时钟(维持现有大厅 UI)。
- 已结束(`day >= totalDays` 或 `room.ended`):`phase = "ended"`,不显示时钟。
- 墙钟偏差(`now < startedAt`):按 day 0 开盘处理(与服务器 clamp 行为一致)。

### UI 改动

- **`Room.tsx` 头部**:替换现有"距下一交易日 MM:SS"倒计时:
  - 盘中:`第3周 · 周四 14:10`
  - 收盘:`第3周 · 周四 · 已收盘 · 距开盘 MM:SS`
  - 周末:`周末休市 · 距开盘 MM:SS`
  - 沿用现有 1 秒 setInterval 刷新。
- **下单区**:`phase` 为 closed/weekend 时加一行小字提示"盘后单,次日开盘成交"。
  下单能力不受任何限制(机制本就是次日开盘成交)。
- **`HeroChart.tsx` 悬停标签**:"第 N 个交易日" → `dayLabel(day)` 的"第W周 · 周X"。
- `roomData.ts` 的 `dayCountdown` 被 simClock 的 `nextOpenSecs` 取代后若无引用则删除。

### 测试(vitest,纯函数单测)

- 跳点间隔均在 [120, 240] 分钟、5 分钟对齐、末点恰为 16:00。
- 确定性:同 `(roomId, day)` 多次调用序列一致;不同 day 序列不同(抽样验证)。
- 墙钟区间切分:78% 边界前后 phase 正确;交易时段内跨片时显示时刻递增。
- 周五 → `phase = "weekend"`;下一日 `weekday` 回到周一、`week` 加一。
- 边界:day 0 起点 9:30;最后一天收盘后 `ended`;`now < startedAt` clamp。

## 不做(YAGNI)

- 法定节假日、时区。
- 盘中价格插值(数据为日粒度,盘中价格不变是预期行为)。
- 服务器任何改动(时钟不入库、不进 API)。
