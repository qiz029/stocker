import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserProviderForTest } from "./testutil";
import Chat from "./Chat";

afterEach(() => vi.restoreAllMocks());

describe("Chat", () => {
  it("renders messages and sends a new one", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      if (init?.method === "POST") {
        posted.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify({ id: 3 }), { status: 200 });
      }
      return new Response(JSON.stringify({ items: [
        { id: 1, username: "host", is_agent: false, day: 0, text: "开局前聊两句", text_en: "" },
        { id: 2, username: "me", is_agent: false, day: 2, text: "冲了", text_en: "" },
        { id: 3, username: "me", is_agent: true, day: 2, text: "我先建仓。", text_en: "I'm opening a position." },
      ] }), { status: 200 });
    });
    render(<UserProviderForTest username="me"><Chat roomId="1" /></UserProviderForTest>);
    expect(await screen.findByText("开局前聊两句")).toBeInTheDocument();
    // own message gets the .me class
    expect(screen.getByText("冲了").closest(".chat-msg")).toHaveClass("me");
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("I'm opening a position.").closest(".chat-msg")).not.toHaveClass("me");

    fireEvent.change(screen.getByPlaceholderText("Say something…"), { target: { value: "科技股什么情况" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(posted).toEqual([{ text: "科技股什么情况" }]));
  });

  it("does not duplicate messages when two overlapping fetches return the same batch", async () => {
    let calls = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async () => {
      calls += 1;
      return new Response(JSON.stringify({ items: [
        { id: 1, username: "host", is_agent: false, day: 0, text: "唯一的一条", text_en: "" },
      ] }), { status: 200 });
    });
    render(<UserProviderForTest username="me"><Chat roomId="1" /></UserProviderForTest>);
    expect(await screen.findByText("唯一的一条")).toBeInTheDocument();
    // Force a second overlapping-style fetch returning the same batch.
    // (fetchNew is internal; trigger via the send path's post-then-fetch)
    fireEvent.change(screen.getByPlaceholderText("Say something…"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(calls).toBeGreaterThan(2));
    expect(screen.getAllByText("唯一的一条")).toHaveLength(1);
  });
});
