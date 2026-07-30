import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, fetchOptions, postDebunk, postHype, postIntel, postLoan, postOptionOrder } from "./api";

const mockFetch = (status: number, body: unknown) =>
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );

afterEach(() => vi.restoreAllMocks());

describe("api client", () => {
  it("returns parsed JSON on success", async () => {
    mockFetch(200, { id: 1, username: "alice" });
    const me = await api.get<{ id: number; username: string }>("/api/me");
    expect(me.username).toBe("alice");
  });

  it("throws ApiError with server message on failure", async () => {
    mockFetch(409, { error: "username taken" });
    await expect(api.post("/api/register", { username: "a", password: "b" }))
      .rejects.toMatchObject({ status: 409, message: "username taken" });
  });

  it("sends JSON bodies with the right content type", async () => {
    const f = mockFetch(200, {});
    await api.post("/api/login", { username: "u", password: "p" });
    const [, init] = f.mock.calls[0]!;
    expect((init!.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(init!.body).toBe(JSON.stringify({ username: "u", password: "p" }));
  });

  it("throws ApiError even when the error body is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("boom", { status: 500 }));
    await expect(api.get("/api/me")).rejects.toBeInstanceOf(ApiError);
  });

  it("postLoan posts action + amount to the loans endpoint", async () => {
    const f = mockFetch(200, { action: "borrow", amount_cents: 500_000,
      cash_cents: 10_500_000, debt_cents: 500_000, max_debt_cents: 20_000_000 });
    const res = await postLoan("7", "borrow", 500_000);
    expect(res.debt_cents).toBe(500_000);
    const [url, init] = f.mock.calls[0]!;
    expect(String(url)).toBe("/api/rooms/7/loans");
    expect(init!.body).toBe(JSON.stringify({ action: "borrow", amount_cents: 500_000 }));
  });

  it("postLoan surfaces the server's 400 message", async () => {
    mockFetch(400, { error: "debt cap exceeded" });
    await expect(postLoan("7", "borrow", 1)).rejects.toMatchObject({ status: 400, message: "debt cap exceeded" });
  });

  it("fetchOptions GETs the chain for one instrument", async () => {
    const f = mockFetch(200, [{ option_id: 11, kind: "call", strike: 120, expiry_day: 10, price: 3.42 }]);
    const res = await fetchOptions("7", "S1");
    expect(res).toHaveLength(1);
    expect(res[0]!.kind).toBe("call");
    expect(String(f.mock.calls[0]![0])).toBe("/api/rooms/7/options?instrument_id=S1");
  });

  it("postOptionOrder posts option_id + action + contracts", async () => {
    const f = mockFetch(200, { action: "buy", contracts: 2, price: 3.42, amount_cents: 684, cash_cents: 9_999_316 });
    const res = await postOptionOrder("7", 11, "buy", 2);
    expect(res.amount_cents).toBe(684);
    const [url, init] = f.mock.calls[0]!;
    expect(String(url)).toBe("/api/rooms/7/options/orders");
    expect(init!.body).toBe(JSON.stringify({ option_id: 11, action: "buy", contracts: 2 }));
  });

  it("postOptionOrder surfaces the server's 400 message", async () => {
    mockFetch(400, { error: "insufficient cash" });
    await expect(postOptionOrder("7", 11, "buy", 100))
      .rejects.toMatchObject({ status: 400, message: "insufficient cash" });
  });

  it("postHype posts instrument_id + direction + tier and parses the outcome", async () => {
    const f = mockFetch(200, { fee_cents: 500_000, caught: false, fine_cents: 0, cash_cents: 9_500_000 });
    const res = await postHype("7", "S1", "up", 1);
    expect(res.caught).toBe(false);
    expect(res.fee_cents).toBe(500_000);
    const [url, init] = f.mock.calls[0]!;
    expect(String(url)).toBe("/api/rooms/7/actions/hype");
    expect(init!.body).toBe(JSON.stringify({ instrument_id: "S1", direction: "up", tier: 1 }));
  });

  it("postHype surfaces the server's 400 message", async () => {
    mockFetch(400, { error: "one hype per day" });
    await expect(postHype("7", "S1", "down", 3))
      .rejects.toMatchObject({ status: 400, message: "one hype per day" });
  });

  it("postDebunk posts news_id and parses the verdict", async () => {
    const f = mockFetch(200, { verdict: "likely_false", fee_cents: 200_000, cash_cents: 9_800_000 });
    const res = await postDebunk("7", 42);
    expect(res.verdict).toBe("likely_false");
    const [url, init] = f.mock.calls[0]!;
    expect(String(url)).toBe("/api/rooms/7/actions/debunk");
    expect(init!.body).toBe(JSON.stringify({ news_id: 42 }));
  });

  it("postDebunk surfaces the server's 400 message", async () => {
    mockFetch(400, { error: "already disputed" });
    await expect(postDebunk("7", 42))
      .rejects.toMatchObject({ status: 400, message: "already disputed" });
  });

  it("postIntel posts instrument_id and parses outlook + nullable strength", async () => {
    const f = mockFetch(200, { outlook: "quiet", strength: null, fee_cents: 300_000, cash_cents: 9_700_000 });
    const res = await postIntel("7", "S1");
    expect(res.outlook).toBe("quiet");
    expect(res.strength).toBeNull();
    const [url, init] = f.mock.calls[0]!;
    expect(String(url)).toBe("/api/rooms/7/actions/intel");
    expect(init!.body).toBe(JSON.stringify({ instrument_id: "S1" }));
  });

  it("postIntel surfaces the server's 400 message", async () => {
    mockFetch(400, { error: "insufficient cash" });
    await expect(postIntel("7", "S1"))
      .rejects.toMatchObject({ status: 400, message: "insufficient cash" });
  });
});
