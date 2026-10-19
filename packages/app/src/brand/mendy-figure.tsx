import type { SVGProps } from "react";

type MendyExpression = "empty" | "in-place" | "new";

/** One shared, non-interactive character asset. Do not morph the body or eye. */
export function MendyFigure({
  expression = "in-place",
  ...props
}: SVGProps<SVGSVGElement> & { expression?: MendyExpression }) {
  return (
    <svg viewBox="0 0 190 302" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <defs>
        <linearGradient id="mendy-case-fill" x1="40" y1="92" x2="166" y2="240" gradientUnits="userSpaceOnUse">
          <stop stopColor="#8EE679" />
          <stop offset="1" stopColor="#7CD56E" />
        </linearGradient>
      </defs>
      <path d="M59 89V55c0-21 15-36 36-36h22c21 0 36 15 36 36v34" stroke="#65BF5B" strokeWidth="17" strokeLinejoin="round"/>
      <path d="m70 229-5 39c-.7 6-5 10-11 10h-9c-7 0-11 4-11 10h45c4 0 7-3 7-7l4-52H70Z" fill="#121212"/>
      <path d="m112 229 4 49c.4 6 5 10 11 10h40c0-7-5-11-12-11h-11l-9-48h-23Z" fill="#121212"/>
      <rect x="25" y="82" width="141" height="156" rx="28" fill="url(#mendy-case-fill)"/>
      <circle cx="139" cy="145" r="33" fill="#FFFFFF"/>
      {expression === "empty" ? (
        <>
          <path d="M114 153c13 0 36 0 50-2" stroke="#D3D7D5" strokeWidth="6" strokeLinecap="round"/>
          <path d="M127 160h31" stroke="#121212" strokeWidth="4" strokeLinecap="round"/>
        </>
      ) : expression === "new" ? (
        <circle cx="151" cy="151" r="13" fill="#FFC954"/>
      ) : (
        <circle cx="151" cy="152" r="14" fill="#121212"/>
      )}
    </svg>
  );
}
