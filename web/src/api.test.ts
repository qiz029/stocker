import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./api";

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
});
