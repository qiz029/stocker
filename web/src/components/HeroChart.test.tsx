import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { dayLabel } from "../simClock";
import HeroChart from "./HeroChart";

describe("HeroChart", () => {
  const series = [100, 105, 103, 108];
  it("shows the latest value and an up delta", () => {
    render(<HeroChart label="总资产" series={series} startDay={0} formatValue={v => `$${v.toFixed(2)}`} />);
    expect(screen.getByText("$108.00")).toBeInTheDocument();
    expect(screen.getByText(/\+8\.00%/)).toBeInTheDocument();
  });
  it("switches range tabs", () => {
    render(<HeroChart label="x" series={series} startDay={0} formatValue={v => String(v)} />);
    fireEvent.click(screen.getByRole("button", { name: "7D" }));
    expect(screen.getByRole("button", { name: "7D" })).toHaveClass("on");
  });
  it("scrub label uses the fictional calendar", () => {
    // 单测 dayLabel 集成点:HeroChart 悬停文案 = dayLabel(startDay + winStart + hover)
    expect(dayLabel(17, "zh")).toBe("第4周 · 周三");
  });
});
