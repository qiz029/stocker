import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { I18nProvider } from "../i18n";
import Docs from "./Docs";

afterEach(() => localStorage.clear());

describe("Docs page", () => {
  it("introduces the complete game loop and switches language", () => {
    render(
      <MemoryRouter initialEntries={["/docs"]}>
        <I18nProvider>
          <Routes><Route path="/docs" element={<Docs />} /></Routes>
        </I18nProvider>
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "How to play Stocker" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "The core loop" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "News is a signal, not the truth" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Advanced moves" })).toBeInTheDocument();
    expect(screen.getByText(/five Agent competitors/i)).toBeInTheDocument();
    expect(screen.getByText(/next market open/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "中文" }));
    expect(screen.getByRole("heading", { name: "Stocker 玩法说明" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "新闻是信号，不是事实" })).toBeInTheDocument();
  });
});
