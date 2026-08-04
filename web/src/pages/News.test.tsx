import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import News from "./News";

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("News detail page", () => {
  it("loads a story directly and renders its full localized content", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async input => {
      const path = String(input);
      if (path === "/api/rooms/1/news/7") {
        return new Response(JSON.stringify({
          id: 7, day: 4, media_id: "wire",
          headline: "S1宣布重大资产重组", body: "这是完整正文。",
          headline_en: "S1 announces a major restructuring",
          body_en: "This is the full article body.",
          disputed: true, exposed: true, cluster_id: 2,
        }), { status: 200 });
      }
      if (path === "/api/rooms/1") {
        return new Response(JSON.stringify({
          room: { id: 1, invite_code: "ABC", scenario_id: "s", days: 30, status: "running", day_duration_secs: 60 },
          instruments: [{ id: "S1", alias: "Ridgeline Networks", desc: "", profile: null }],
          quotes: [], leaderboard: [],
        }), { status: 200 });
      }
      return new Response(JSON.stringify({ error: "unexpected request" }), { status: 500 });
    });

    render(
      <MemoryRouter initialEntries={["/rooms/1/news/7"]}>
        <I18nProvider>
          <Routes><Route path="/rooms/:roomId/news/:newsId" element={<News />} /></Routes>
        </I18nProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Ridgeline Networks announces a major restructuring" })).toBeInTheDocument();
    expect(screen.getByText("This is the full article body.")).toBeInTheDocument();
    expect(screen.getByText("Disputed")).toBeInTheDocument();
    expect(screen.getByText("Manipulation confirmed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "← Back to room" })).toHaveAttribute("href", "/rooms/1");
  });
});
