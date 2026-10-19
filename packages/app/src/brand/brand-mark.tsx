import type { SVGProps } from "react";

/**
 * Mender's one-port case mark, shared by the navigation and product surfaces.
 * The Care Green body and Ink port are intentionally never theme-tinted.
 */
export function BrandMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 96 96" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path
        d="M75 28v-2c0-9-7-16-16-16H29c-10 0-17 7-17 17v42c0 10 7 17 17 17h40"
        stroke="#7CD56E"
        strokeWidth="11"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="77" cy="50" r="11" fill="#121212" />
    </svg>
  );
}
