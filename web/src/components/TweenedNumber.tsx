import { useTweenedNumber } from "../useTweenedNumber";

/**
 * Renders `format(value)` with the number easing toward the latest value on
 * each update, so polled figures (cash, totals) glide instead of jumping.
 */
export default function TweenedNumber({ value, format }: { value: number; format: (v: number) => string }) {
  return <>{format(useTweenedNumber(value))}</>;
}
