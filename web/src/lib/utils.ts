import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// FILLED_FOCUS shows keyboard focus on a control filled with a color, e.g., a
// primary button or a checked checkbox, in place of the focus ring. A ring
// touches the control and merges with a fill close to its color, so this is an
// outline a gap away, and the gap shows whatever surface is under the control.
// The control also takes `outline-none`.
export const FILLED_FOCUS =
  "focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-solid focus-visible:outline-(--color-ring)";
