# 模拟市场时钟 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把机械的"距下一交易日 MM:SS"换成有市场质感的模拟时钟:盘中 2-4 小时一跳(9:30→16:00)、收盘时段、周五后"周末休市",配虚构日历"第W周 · 周X"。

**Architecture:** 纯前端展示层。新增纯函数模块 `web/src/simClock.ts` 把 `(started_at, day_duration_secs, roomId, days, now)` 映射为时钟状态;`roomData.ts` 加一个 `useSimClock` hook(1 秒刷新);Room 头部、HeroChart 悬停标签、TradePanel 盘后提示消费它。服务器零改动。

**Tech Stack:** React 18 + TypeScript,vitest + @testing-library/react。测试命令都在 `web/` 下运行。

**Spec:** `docs/superpowers/specs/2026-07-27-sim-market-clock-design.md`

## Global Constraints

- 服务器/API 零改动;时钟不入库。
- 收盘/周末不限制下单(机制本就是次日开盘成交)。
- 不用真实历史日期(盲盒防剧透),用虚构日历 `第W周 · 周X`。
- 交易时段占墙钟日的前 78%(`SESSION_FRAC = 0.78`),收盘/周末占后 22%。
- 盘中跳点:9:30(570 分)到 16:00(960 分),间隔 120~240 分钟、5 分钟对齐,由 `hash(roomId, day)` 确定性生成(所有玩家看到同一时钟)。
- 每个任务完成后 `cd web && npm test && npm run typecheck` 必须全绿再提交。
- 提交信息末尾加 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: simClock 纯函数模块

**Files:**
- Create: `web/src/simClock.ts`
- Test: `web/src/simClock.test.ts`

**Interfaces:**
- Consumes: 无(纯新增)。
- Produces(后续任务依赖的精确签名):
  - `type ClockPhase = "open" | "closed" | "weekend" | "ended"`
  - `interface SimClockState { day: number; week: number; weekday: number; dateLabel: string; time: string | null; phase: ClockPhase; nextOpenSecs: number | null }`
  - `function simClock(startedAt: string, dayDurationSecs: number, roomId: number, totalDays: number, nowMs?: number): SimClockState`
  - `function dayLabel(day: number): string` — 如 `"第3周 · 周四"`
  - `function sessionTimes(roomId: number, day: number): number[]` — 分钟数序列,首 570 末 960
  - `const SESSION_FRAC = 0.78`

- [ ] **Step 1: 写失败测试**

创建 `web/src/simClock.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { dayLabel, sessionTimes, simClock } from "./simClock";

describe("sessionTimes", () => {
  it("starts 9:30, ends 16:00, gaps are 2-4h on 5-min steps", () => {
    for (let day = 0; day < 200; day++) {
      const t = sessionTimes(7, day);
      expect(t[0]).toBe(570);
      expect(t[t.length - 1]).toBe(960);
      for (let i = 1; i < t.length; i++) {
        const gap = t[i]! - t[i - 1]!;
        expect(gap).toBeGreaterThanOrEqual(120);
        expect(gap).toBeLessThanOrEqual(240);
        expect(gap % 5).toBe(0);
      }
    }
  });

  it("is deterministic per (roomId, day) and varies across days", () => {
    expect(sessionTimes(7, 3)).toEqual(sessionTimes(7, 3));
    const distinct = new Set(
      Array.from({ length: 50 }, (_, d) => sessionTimes(7, d).join(",")));
    expect(distinct.size).toBeGreaterThan(1);
  });
});

describe("dayLabel", () => {
  it("maps a day index to the fictional calendar", () => {
    expect(dayLabel(0)).toBe("第1周 · 周一");
    expect(dayLabel(4)).toBe("第1周 · 周五");
    expect(dayLabel(5)).toBe("第2周 · 周一");
    expect(dayLabel(17)).toBe("第4周 · 周三");
  });
});

describe("simClock", () => {
  // D = 100 s/day → 交易时段 78 s,收盘 22 s;100_000 ms 一天
  const t0 = Date.parse("2026-07-26T12:00:00Z");
  const started = new Date(t0).toISOString();
  const D = 100;

  it("opens at 9:30 and clamps clock skew to day 0", () => {
    expect(simClock(started, D, 1, 10, t0).time).toBe("9:30");
    expect(simClock(started, D, 1, 10, t0 - 5000).phase).toBe("open");
  });

  it("clock is non-decreasing within a session and hits 16:00 last", () => {
    let prev = 0;
    for (let ms = 0; ms < 78_000; ms += 500) {
      const c = simClock(started, D, 1, 10, t0 + ms);
      expect(c.phase).toBe("open");
      const [h, m] = c.time!.split(":").map(Number);
      const mins = h! * 60 + m!;
      expect(mins).toBeGreaterThanOrEqual(prev);
      prev = mins;
    }
    expect(prev).toBe(960); // 16:00
  });

  it("closes after 78% of the wall day with a next-open countdown", () => {
    const c = simClock(started, D, 1, 10, t0 + 78_000);
    expect(c.phase).toBe("closed"); // day 0 = 周一
    expect(c.time).toBeNull();
    expect(c.nextOpenSecs).toBe(22);
    expect(c.dateLabel).toBe("第1周 · 周一");
  });

  it("friday close is a weekend; next day is monday of week 2", () => {
    const fri = simClock(started, D, 1, 10, t0 + 4 * 100_000 + 90_000);
    expect(fri.phase).toBe("weekend");
    const mon = simClock(started, D, 1, 10, t0 + 5 * 100_000);
    expect(mon.phase).toBe("open");
    expect(mon.dateLabel).toBe("第2周 · 周一");
  });

  it("is ended once elapsed passes totalDays", () => {
    const c = simClock(started, D, 1, 10, t0 + 10 * 100_000);
    expect(c.phase).toBe("ended");
    expect(c.time).toBeNull();
    expect(c.nextOpenSecs).toBeNull();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && npx vitest run src/simClock.test.ts`
Expected: FAIL —— `Cannot find module './simClock'`(或等价的解析错误)。

- [ ] **Step 3: 实现 `web/src/simClock.ts`**

```ts
/**
 * 模拟市场时钟:把墙钟在每个交易日区间内的进度映射为虚构日历
 * ("第W周 · 周X")和盘中 2-4 小时一跳的时钟(9:30→16:00),之后是
 * 收盘/周末时段。纯展示层,服务器的线性 day 映射不受影响。
 */

export type ClockPhase = "open" | "closed" | "weekend" | "ended";

export interface SimClockState {
  day: number;
  week: number;                // 1 起始
  weekday: number;             // 0..4 → 周一..周五
  dateLabel: string;           // "第3周 · 周四"
  time: string | null;         // 盘中 "14:10",其余 null
  phase: ClockPhase;
  nextOpenSecs: number | null; // closed/weekend 时距下一开盘秒数
}

export const SESSION_FRAC = 0.78;

const OPEN_MIN = 9 * 60 + 30;            // 570
const CLOSE_MIN = 16 * 60;               // 960
const TOTAL_MIN = CLOSE_MIN - OPEN_MIN;  // 390
const MIN_GAP = 120;
const MAX_GAP = 240;
const WEEKDAYS = ["一", "二", "三", "四", "五"];

// mulberry32:小型确定性 PRNG,返回 [0,1)
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// 在 [lo, hi](均为 5 的倍数)中均匀取一个 5 的倍数
function pick5(rng: () => number, lo: number, hi: number): number {
  const count = (hi - lo) / 5 + 1;
  return lo + 5 * Math.min(count - 1, Math.floor(rng() * count));
}

/**
 * (roomId, day) 的盘中跳点,分钟数(自午夜)。首项 570(9:30),
 * 末项恰为 960(16:00),相邻间隔 120~240 分钟、5 分钟对齐。
 */
export function sessionTimes(roomId: number, day: number): number[] {
  const seed = (Math.imul(roomId, 2654435761) ^ Math.imul(day + 1, 40503)) >>> 0;
  const rng = mulberry32(seed);
  const n = rng() < 0.5 ? 2 : 3; // 跳数
  const times = [OPEN_MIN];
  let remaining = TOTAL_MIN;
  for (let k = n; k >= 1; k--) {
    // 留给后面 k-1 段的余量决定本段可行区间,保证末点恰为 16:00
    const gap = k === 1
      ? remaining
      : pick5(rng,
          Math.max(MIN_GAP, remaining - MAX_GAP * (k - 1)),
          Math.min(MAX_GAP, remaining - MIN_GAP * (k - 1)));
    times.push(times[times.length - 1]! + gap);
    remaining -= gap;
  }
  return times;
}

export function dayLabel(day: number): string {
  return `第${Math.floor(day / 5) + 1}周 · 周${WEEKDAYS[day % 5]}`;
}

function fmtMin(mins: number): string {
  return `${Math.floor(mins / 60)}:${String(mins % 60).padStart(2, "0")}`;
}

export function simClock(
  startedAt: string,
  dayDurationSecs: number,
  roomId: number,
  totalDays: number,
  nowMs: number = Date.now(),
): SimClockState {
  const D = dayDurationSecs;
  const elapsed = Math.max(0, (nowMs - new Date(startedAt).getTime()) / 1000);
  let day = Math.floor(elapsed / D);
  const ended = day >= totalDays;
  if (ended) day = totalDays - 1;
  const weekday = day % 5;
  const base = {
    day, weekday,
    week: Math.floor(day / 5) + 1,
    dateLabel: dayLabel(day),
  };
  if (ended) return { ...base, time: null, phase: "ended", nextOpenSecs: null };

  const into = elapsed - day * D;
  const sessionSecs = D * SESSION_FRAC;
  if (into < sessionSecs) {
    const times = sessionTimes(roomId, day);
    const slice = Math.min(times.length - 1,
      Math.floor((into / sessionSecs) * times.length));
    return { ...base, time: fmtMin(times[slice]!), phase: "open", nextOpenSecs: null };
  }
  return {
    ...base, time: null,
    phase: weekday === 4 ? "weekend" : "closed",
    nextOpenSecs: Math.round(D - into),
  };
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && npx vitest run src/simClock.test.ts`
Expected: PASS(6 个用例)。

- [ ] **Step 5: 提交**

```bash
git add web/src/simClock.ts web/src/simClock.test.ts
git commit -m "feat(web): sim market clock core — 2-4h intraday jumps, close/weekend phases

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: useSimClock hook + Room 头部时钟(替换旧倒计时)

**Files:**
- Modify: `web/src/roomData.ts`(加 `useSimClock`,删 `dayCountdown`)
- Modify: `web/src/roomData.test.ts`(删 `dayCountdown` 用例)
- Modify: `web/src/pages/Room.tsx`(头部换时钟)
- Test: `web/src/pages/Room.test.tsx`(新增时钟渲染用例)

**Interfaces:**
- Consumes: Task 1 的 `simClock`、`SimClockState`。
- Produces: `function useSimClock(room: Room | undefined): SimClockState | null`(roomData.ts 导出;房间未启动/已结束返回 null,否则每秒刷新)。Task 4 依赖它。

- [ ] **Step 1: 写失败测试**

`web/src/pages/Room.test.tsx` 的 describe 块内新增用例(复用文件顶部已有的 `state`/`portfolio`/`mockApi`;started_at 用相对当前时间,落在 day 0 的交易时段):

```tsx
  it("shows the sim market clock while a day is open", async () => {
    mockApi();
    const openState = {
      ...state,
      room: { ...state.room, started_at: new Date(Date.now() - 1000).toISOString(), current_day: 0 },
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = openState;
      else if (u.endsWith("/portfolio")) body = portfolio;
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/prices/")) body = priceDays;
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    // day 0 开盘:虚构日历 + 盘中时刻(9:30 起跳)
    await waitFor(() => {
      const el = document.querySelector(".countdown");
      expect(el?.textContent).toContain("第1周 · 周一");
      expect(el?.textContent).toMatch(/\d{1,2}:\d{2}/);
    });
  });
```

注意:`mockApi()` 在这里只是保底,随后的 `vi.spyOn` 会覆盖它;直接只写第二个 mock 也可以——保持和文件中现有用例一致的风格即可。

同时删除 `web/src/roomData.test.ts` 中 `computes seconds until next trading day` 用例及其 `dayCountdown` import(该函数将被移除)。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && npx vitest run src/pages/Room.test.tsx`
Expected: FAIL —— `.countdown` 内容是"距下一交易日…",不含"第1周 · 周一"。

- [ ] **Step 3: 实现 hook 与头部**

`web/src/roomData.ts`:

1. 删除 `dayCountdown` 函数(第 14-19 行)。
2. import 区加:`import { SimClockState, simClock } from "./simClock";`,并把 `Room` 加入 `./api` 的 import。
3. 文件末尾新增:

```ts
/** 房间运行中每秒刷新的模拟市场时钟;未启动或已结束时为 null。 */
export function useSimClock(room: Room | undefined): SimClockState | null {
  const [clock, setClock] = useState<SimClockState | null>(null);
  const startedAt = room?.started_at;
  const ended = room?.ended ?? false;
  const durationSecs = room?.day_duration_secs;
  const roomId = room?.id;
  const days = room?.days;
  useEffect(() => {
    if (!startedAt || ended || !durationSecs || !roomId || !days) {
      setClock(null);
      return;
    }
    const update = () => setClock(simClock(startedAt, durationSecs, roomId, days));
    update();
    const t = setInterval(update, 1000);
    return () => clearInterval(t);
  }, [startedAt, ended, durationSecs, roomId, days]);
  return clock;
}
```

`web/src/pages/Room.tsx`:

1. import 改为 `import { useRoomData, useSimClock } from "../roomData";`(去掉 `dayCountdown`);`useState`/`useEffect` 删除倒计时逻辑后在本文件已无使用,整行 `import { useEffect, useState } from "react";` 删除。
2. 删除 `countdown` state 与整个 `useEffect`(第 18、21-27 行),换成:

```tsx
  const clock = useSimClock(state?.room);
```

(放在 `const room = state?.room;` 之后。)

3. 删除 `mmss` 常量(第 50-52 行),换成模块级辅助(组件函数外):

```tsx
const mmss = (s: number) =>
  `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
```

4. 头部第 65 行 `{mmss && <div className="countdown">…</div>}` 替换为:

```tsx
        {clock && clock.phase !== "ended" && (
          <div className="countdown">
            {clock.phase === "open"
              ? <>{clock.dateLabel} <b className="num">{clock.time}</b></>
              : <>
                  {clock.phase === "weekend" ? "周末休市" : `${clock.dateLabel} · 已收盘`}
                  {" · 距开盘 "}<b className="num">{mmss(clock.nextOpenSecs!)}</b>
                </>}
          </div>
        )}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && npm test && npm run typecheck`
Expected: 全部 PASS(含既有 Room 用例:老 mock 的 started_at 是固定过去时刻,推算已超 totalDays → phase "ended" → 不渲染时钟,不影响原断言)。

- [ ] **Step 5: 提交**

```bash
git add web/src/roomData.ts web/src/roomData.test.ts web/src/pages/Room.tsx web/src/pages/Room.test.tsx
git commit -m "feat(web): room header shows sim market clock instead of mechanical countdown

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: HeroChart 悬停标签用虚构日历

**Files:**
- Modify: `web/src/components/HeroChart.tsx:107-109`
- Test: `web/src/components/HeroChart.test.tsx`

**Interfaces:**
- Consumes: Task 1 的 `dayLabel(day: number): string`。
- Produces: 无新接口(仅显示变化)。

- [ ] **Step 1: 写失败测试**

`web/src/components/HeroChart.test.tsx` 新增用例(沿用该文件现有 render 方式;若现有用例已有 render 帮助函数就复用)。悬停由 `onPointerMove` 触发,jsdom 下 `getBoundingClientRect` 返回全 0,`(clientX - 0) / 0` 不可靠——改为直接断言"未悬停时占位符仍渲染 + dayLabel 集成":

```tsx
import { dayLabel } from "../simClock";

it("scrub label uses the fictional calendar", () => {
  // 单测 dayLabel 集成点:HeroChart 悬停文案 = dayLabel(startDay + winStart + hover)
  expect(dayLabel(17)).toBe("第4周 · 周三");
});
```

同时把 HeroChart.tsx 的改动断言交给 typecheck + 手测(悬停是 canvas 交互,jsdom 覆盖成本高,YAGNI)。

- [ ] **Step 2: 修改 HeroChart**

`web/src/components/HeroChart.tsx`:

1. import 加:`import { dayLabel } from "../simClock";`
2. 第 107-109 行:

```tsx
        <div className="scrub-date num">
          {hover !== null ? dayLabel(startDay + winStart + hover) : " "}
        </div>
```

- [ ] **Step 3: 跑测试**

Run: `cd web && npm test && npm run typecheck`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add web/src/components/HeroChart.tsx web/src/components/HeroChart.test.tsx
git commit -m "feat(web): chart scrub label uses fictional calendar dates

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: TradePanel 盘后提示 + Stock 页接线

**Files:**
- Modify: `web/src/components/TradePanel.tsx`
- Modify: `web/src/pages/Stock.tsx`
- Test: `web/src/components/TradePanel.test.tsx`

**Interfaces:**
- Consumes: Task 2 的 `useSimClock(room)`;Task 1 的 `SimClockState.phase`。
- Produces: `TradePanel` 新增可选 prop `afterHours?: boolean`。

- [ ] **Step 1: 写失败测试**

`web/src/components/TradePanel.test.tsx` 新增用例(复用该文件现有的 render/props 模式,portfolio 传 null 即可):

```tsx
it("shows the after-hours note when the market is closed", () => {
  render(<TradePanel roomId="1" instrumentId="S1" lastClose={100}
    portfolio={null} onChanged={() => {}} afterHours />);
  expect(screen.getByText(/已收盘.*次日开盘成交/)).toBeInTheDocument();
});

it("hides the after-hours note while the market is open", () => {
  render(<TradePanel roomId="1" instrumentId="S1" lastClose={100}
    portfolio={null} onChanged={() => {}} />);
  expect(screen.queryByText(/已收盘/)).not.toBeInTheDocument();
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && npx vitest run src/components/TradePanel.test.tsx`
Expected: FAIL —— ts/prop 不存在或文案找不到。

- [ ] **Step 3: 实现**

`web/src/components/TradePanel.tsx`:

1. Props 加 `afterHours?: boolean;`,解构处加 `afterHours`。
2. 第 96 行 note 下方新增:

```tsx
      {afterHours && <p className="note">现在已收盘：盘后单照常受理，次日开盘成交。</p>}
```

`web/src/pages/Stock.tsx`:

1. import 改:`import { useRoomData, useSimClock } from "../roomData";`
2. `const { state, ... } = useRoomData(roomId!);` 之后加:

```tsx
  const clock = useSimClock(state?.room);
```

(注意放在 `if (!state) return null;` 之前——hook 不能条件调用。)

3. TradePanel 调用处加 prop:

```tsx
        <TradePanel roomId={roomId!} instrumentId={instrumentId!} lastClose={last}
          portfolio={portfolio} onChanged={reload}
          afterHours={clock?.phase === "closed" || clock?.phase === "weekend"} />
```

- [ ] **Step 4: 跑全量测试**

Run: `cd web && npm test && npm run typecheck`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add web/src/components/TradePanel.tsx web/src/components/TradePanel.test.tsx web/src/pages/Stock.tsx
git commit -m "feat(web): after-hours order note in trade panel during close/weekend

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## 完成校验(整体)

- [ ] `cd web && npm test && npm run typecheck` 全绿。
- [ ] 手测(可选,`./run-local.sh` 起本地环境):建房用 60 秒/日,观察头部时钟 9:30 起跳、约 47 秒(78%)后显示"已收盘 · 距开盘",第 5 天收盘显示"周末休市",随后日历跳到"第2周 · 周一"。
- [ ] 服务器目录 `server/` 无任何 diff:`git status server/` 干净。
