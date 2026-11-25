export function IslandPostmark() {
  return (
    <svg className="postcard-postmark" viewBox="0 0 148 82" fill="none" aria-hidden="true">
      <g stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="42" cy="41" r="34" />
        <circle cx="42" cy="41" r="28" strokeDasharray="2 4" />
        <path d="m25 45 13-16 9 11 5-6 9 11M24 51q5-3 10 0t10 0 10 0 8 0" />
        <circle cx="52" cy="26" r="3" />
        <path d="M70 29c12-7 19-7 31-2s23 7 41-2M72 40c12-7 19-7 31-2s23 7 41-2M71 51c12-7 19-7 31-2s23 7 41-2" />
      </g>
    </svg>
  );
}

// The unbranded product illustration uses the same flat inks as the storefront.
export function IslandVisual({ variant = 0 }: { variant?: number }) {
  const wrap = ["#ffd37d", "#ffa684", "#b4dcf0", "#a6dcc1"][variant % 4];
  return (
    <svg className="island-visual" viewBox="0 0 132 124" fill="none" aria-hidden="true">
      <g stroke="#283f39" strokeWidth="3.1" strokeLinecap="round" strokeLinejoin="round">
        <rect x="4" y="4" width="124" height="116" rx="23" fill="#fff9e8" />
        <path d="M20 92q10-7 21 0t21 0 21 0 21 0" stroke="#458f84" />
        <path d="m32 45 35-13 32 12 1 38-32 13-37-15z" fill={wrap} />
        <path d="m32 45 35 13 32-14M67 58l1 37" />
        <path d="m44 40 34 13 10-4-34-13z" fill="#fa9873" />
        <path d="m77 54 11-5v15l-5-2-5 5z" fill="#fa9873" />
        <path d="M63 35C45 26 53 17 60 22c5 3 7 10 7 12 3-8 8-15 14-13 9 5-4 13-14 15" fill="#f3916b" />
        <path d="m27 25 3-5 2 5 5 2-5 2-2 5-3-5-4-2z" fill="#f7bd50" strokeWidth="2" />
        <path d="M96 103q5-3 11-2M23 101l7 2" stroke="#458f84" strokeWidth="2" />
      </g>
    </svg>
  );
}
