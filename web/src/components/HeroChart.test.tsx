import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
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
    fireEvent.click(screen.getByRole("button", { name: "7日" }));
    expect(screen.getByRole("button", { name: "7日" })).toHaveClass("on");
  });
});
