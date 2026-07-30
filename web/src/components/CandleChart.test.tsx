import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import CandleChart from "./CandleChart";

// jsdom has no 2d canvas context — the draw effect bails, so these tests
// cover the DOM surface: latest values, OHLC readout, hover data path, tabs.
const days = [
  { open: 100, high: 110, low: 95, close: 105 },
  { open: 105, high: 115, low: 100, close: 90 },
  { open: 90, high: 100, low: 85, close: 98 },
];
const fmt = (v: number) => `$${v.toFixed(2)}`;

afterEach(() => vi.restoreAllMocks());

describe("CandleChart", () => {
  it("shows the latest close, delta and OHLC readout", () => {
    render(<CandleChart label="S1" days={days} formatValue={fmt} />);
    expect(screen.getByText("$98.00")).toBeInTheDocument();
    expect(screen.getByText(/O \$90\.00 · H \$100\.00 · L \$85\.00 · C \$98\.00/)).toBeInTheDocument();
    // down vs window start (105 → 98)
    expect(screen.getByText(/-6\.67%/)).toBeInTheDocument();
  });

  it("hover crosshair shows that day's O/H/L/C and date", () => {
    vi.spyOn(HTMLCanvasElement.prototype, "getBoundingClientRect")
      .mockReturnValue({ left: 0, top: 0, width: 300, height: 100,
        right: 300, bottom: 100, x: 0, y: 0, toJSON: () => ({}) } as DOMRect);
    const { container } = render(<CandleChart label="S1" days={days} formatValue={fmt} />);
    const canvas = container.querySelector("canvas")!;
    // jsdom has no PointerEvent constructor; a MouseEvent typed "pointermove"
    // carries clientX and still hits React's onPointerMove.
    // left edge → first day of the window
    fireEvent(canvas, new MouseEvent("pointermove", { bubbles: true, clientX: 0 }));
    expect(screen.getByText(/O \$100\.00 · H \$110\.00 · L \$95\.00 · C \$105\.00/)).toBeInTheDocument();
    expect(screen.getByText("Week 1 · Mon")).toBeInTheDocument();
    fireEvent.pointerLeave(canvas);
    expect(screen.getByText(/C \$98\.00/)).toBeInTheDocument();
  });

  it("switches range tabs", () => {
    render(<CandleChart label="S1" days={days} formatValue={fmt} />);
    fireEvent.click(screen.getByRole("button", { name: "7D" }));
    expect(screen.getByRole("button", { name: "7D" })).toHaveClass("on");
  });
});
