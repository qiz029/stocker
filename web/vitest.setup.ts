import "@testing-library/jest-dom/vitest";

// jsdom has no canvas; stub the 2D context so chart components can mount.
const noop = () => undefined;
const mockGradient = {
  addColorStop: noop,
};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(HTMLCanvasElement.prototype as any).getContext = function () {
  return new Proxy(
    { canvas: this, createLinearGradient: () => mockGradient },
    { get: (t, p) => (p in t ? (t as never)[p] : noop) },
  );
};
