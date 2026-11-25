export function QingyuMark({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 40 40"
      fill="none"
      aria-hidden="true"
    >
      <rect width="40" height="40" rx="11" fill="currentColor" />
      <rect
        x="1"
        y="1"
        width="38"
        height="38"
        rx="10"
        stroke="#FFFFFF"
        strokeOpacity="0.16"
      />
      <circle cx="28.5" cy="11.5" r="1.8" fill="#E8FAF8" />
      <path
        d="M7 23.5C11 23.5 13 12.5 18 12.5C22.5 12.5 24.5 23.5 29 23.5Z"
        fill="#F6FBFA"
      />
      <path
        d="M19.5 23.5C22 23.5 24 16.2 27.5 16.2C30.5 16.2 31.8 23.5 33.5 23.5Z"
        fill="#8CDAD1"
      />
      <path
        d="M6.5 27.2Q12 24.7 17.5 27.2T28.5 27.2T33.5 27.2"
        stroke="#F6FBFA"
        strokeWidth="2.4"
        strokeLinecap="round"
      />
      <path
        d="M9.5 32.2Q15 29.7 20.5 32.2T30.5 32.2"
        stroke="#8CDAD1"
        strokeWidth="2.1"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function IslandVisual({ variant = 0 }: { variant?: number }) {
  const palettes = [
    { deep: "#0B3E44", mid: "#0F4C52", light: "#196F77", foam: "#8CDAD1", glow: "#DDF5F2" },
    { deep: "#0C424D", mid: "#135863", light: "#1F7C86", foam: "#95DFD8", glow: "#E1F6F5" },
    { deep: "#0E4643", mid: "#155D58", light: "#238077", foam: "#8FE0CE", glow: "#E0F7F1" },
    { deep: "#103D4C", mid: "#175264", light: "#257084", foam: "#96D6E3", glow: "#E4F3F8" },
  ];
  const p = palettes[variant % palettes.length];
  const uid = `qy-${variant % palettes.length}`;
  return (
    <svg
      className="island-visual"
      viewBox="0 0 480 460"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={`${uid}-sea`} x1="70" y1="60" x2="400" y2="390" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor={p.deep} />
          <stop offset=".55" stopColor={p.mid} />
          <stop offset="1" stopColor={p.light} />
        </linearGradient>
        <linearGradient id={`${uid}-peak`} x1="180" y1="120" x2="210" y2="275" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#FFFFFF" />
          <stop offset="1" stopColor="#DCF3F0" />
        </linearGradient>
        <radialGradient id={`${uid}-beacon`} cx=".5" cy=".5" r=".5">
          <stop offset="0" stopColor="#FFFFFF" stopOpacity=".95" />
          <stop offset=".45" stopColor={p.foam} stopOpacity=".45" />
          <stop offset="1" stopColor={p.foam} stopOpacity="0" />
        </radialGradient>
        <filter id={`${uid}-shadow`} x="-25%" y="-25%" width="150%" height="150%">
          <feDropShadow dx="0" dy="18" stdDeviation="16" floodColor={p.deep} floodOpacity=".18" />
        </filter>
      </defs>

      <circle cx="240" cy="226" r="196" fill="none" stroke={p.mid} strokeOpacity=".1" strokeWidth="1" />
      <circle cx="240" cy="226" r="164" fill="none" stroke={p.mid} strokeOpacity=".12" strokeDasharray="5 7" strokeWidth="1" />

      <g filter={`url(#${uid}-shadow)`}>
        <rect x="76" y="64" width="328" height="328" rx="84" fill={`url(#${uid}-sea)`} />
        <rect x="82" y="70" width="316" height="316" rx="78" fill="none" stroke="#FFFFFF" strokeOpacity=".15" strokeWidth="2" />
      </g>

      <circle cx="316" cy="148" r="36" fill={`url(#${uid}-beacon)`} />
      <circle cx="316" cy="148" r="11" fill="#F4FCFB" opacity=".94" />

      <path
        d="M126 260 C162 260 180 156 222 156 C262 156 280 260 318 260 Z"
        fill={`url(#${uid}-peak)`}
      />
      <path
        d="M236 260 C258 260 274 192 308 192 C334 192 346 260 362 260 Z"
        fill={p.foam}
        opacity=".92"
      />
      <path
        d="M158 260 C178 260 192 214 218 214 C242 214 256 260 276 260 Z"
        fill={p.glow}
        opacity=".35"
      />

      <path
        d="M120 286 Q160 268 200 286 T280 286 T360 286"
        stroke="#FFFFFF"
        strokeWidth="12"
        strokeLinecap="round"
        fill="none"
      />
      <path
        d="M144 322 Q184 304 224 322 T304 322 T344 322"
        stroke={p.foam}
        strokeWidth="10"
        strokeLinecap="round"
        fill="none"
      />
      <path
        d="M180 352 Q212 340 244 352 T304 352"
        stroke={p.glow}
        strokeOpacity=".55"
        strokeWidth="6"
        strokeLinecap="round"
        fill="none"
      />
    </svg>
  );
}
