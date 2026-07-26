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
        { id: 1, username: "host", day: 0, text: "开局前聊两句" },
        { id: 2, username: "me", day: 2, text: "冲了" },
      ] }), { status: 200 });
    });
    render(<UserProviderForTest username="me"><Chat roomId="1" /></UserProviderForTest>);
    expect(await screen.findByText("开局前聊两句")).toBeInTheDocument();
    // own message gets the .me class
    expect(screen.getByText("冲了").closest(".chat-msg")).toHaveClass("me");

    fireEvent.change(screen.getByPlaceholderText("说点什么…"), { target: { value: "科技股什么情况" } });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    await waitFor(() => expect(posted).toEqual([{ text: "科技股什么情况" }]));
  });
});
