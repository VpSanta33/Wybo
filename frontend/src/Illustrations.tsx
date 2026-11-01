export function BunnyMark({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 40 40"
      fill="none"
      aria-hidden="true"
    >
      <rect width="40" height="40" rx="12" fill="currentColor" />
      <path
        d="M14.7 18.2c-1.3-3.8-2-9.4.3-9.8 2.6-.4 4.5 5.1 4.8 8.1 1-.1 1.8-.1 2.6.1.6-3.5 2.2-8.6 4.5-7.9 2.2.7.7 6.3-.9 10a8.2 8.2 0 0 1 3.7 6.5c0 4.8-4.3 7.4-9.6 7.4-5.1 0-9.3-2.7-9.3-7.4 0-3 1.5-5.5 3.9-7Z"
        fill="white"
      />
      <path
        d="M22.7 26.5a5.4 5.4 0 0 1-7.2-5.6 5.6 5.6 0 1 0 7.2 5.6Z"
        fill="currentColor"
      />
      <circle cx="24.5" cy="23" r="1" fill="currentColor" />
    </svg>
  );
}

export function MoonRabbit({ variant = 0 }: { variant?: number }) {
  const colors = [
    ["#277bff", "#0050e5", "#0047bd"],
    ["#6f79ff", "#5754e8", "#4342bc"],
    ["#2c99ea", "#0874d9", "#0054a6"],
    ["#7772ef", "#5c4add", "#3f32ae"],
  ][variant % 4];
  return (
    <svg
      className="moon-rabbit"
      viewBox="0 0 480 460"
      role="img"
      aria-label="一只蓝色的小兔子，抱着弯弯的月亮"
    >
      <defs>
        <linearGradient
          id="rabbit-blue"
          x1="170"
          y1="105"
          x2="340"
          y2="370"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor={colors[0]} />
          <stop offset=".5" stopColor={colors[1]} />
          <stop offset="1" stopColor={colors[2]} />
        </linearGradient>
        <linearGradient
          id="moon-blue"
          x1="90"
          y1="100"
          x2="360"
          y2="375"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#8cbaff" />
          <stop offset=".4" stopColor="#bed8ff" />
          <stop offset="1" stopColor="#649eff" />
        </linearGradient>
        <linearGradient
          id="ear-shine"
          x1="170"
          y1="82"
          x2="210"
          y2="229"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#c7e2ff" stopOpacity=".72" />
          <stop offset="1" stopColor="#9ec9ff" stopOpacity="0" />
        </linearGradient>
        <linearGradient
          id="moon-shine"
          x1="100"
          y1="180"
          x2="350"
          y2="330"
          gradientUnits="userSpaceOnUse"
        >
          <stop stopColor="#e5f0ff" />
          <stop offset="1" stopColor="#a7caff" />
        </linearGradient>
        <radialGradient id="bunny-highlight" cx=".35" cy=".18" r=".8">
          <stop stopColor="#8ac4ff" stopOpacity=".58" />
          <stop offset="1" stopColor="#0055e7" stopOpacity="0" />
        </radialGradient>
        <filter id="rabbit-shadow" x="-50%" y="-50%" width="200%" height="220%">
          <feDropShadow
            dx="0"
            dy="16"
            stdDeviation="13"
            floodColor="#195cd6"
            floodOpacity=".17"
          />
        </filter>
        <filter id="ground-blur">
          <feGaussianBlur stdDeviation="9" />
        </filter>
      </defs>
      <ellipse
        cx="248"
        cy="405"
        rx="103"
        ry="9"
        fill="#9bb9ed"
        opacity=".2"
        filter="url(#ground-blur)"
      />
      <g filter="url(#rabbit-shadow)">
        <path
          d="M149 100c-52 30-82 89-70 151 17 85 99 139 182 119 53-12 95-55 106-106-26 37-75 54-121 38-63-21-96-90-74-152 6-18 17-34 30-47-18-9-37-10-53-3Z"
          fill="url(#moon-blue)"
        />
        <path
          d="M148 109c-43 29-69 79-59 131"
          fill="none"
          stroke="url(#moon-shine)"
          strokeWidth="6"
          strokeLinecap="round"
          opacity=".85"
        />
        <path
          d="M165 224c-20-27-39-98-31-133 3-14 12-20 23-16 27 10 40 77 46 112 11-42 28-105 54-111 15-4 23 6 24 20 3 37-22 97-38 128 25 16 40 43 40 73 0 50-37 80-85 80-47 0-85-30-85-78 0-33 21-61 52-75Z"
          fill="url(#rabbit-blue)"
        />
        <path
          d="M165 224c-20-27-39-98-31-133 3-14 12-20 23-16 27 10 40 77 46 112 11-42 28-105 54-111 15-4 23 6 24 20 3 37-22 97-38 128 25 16 40 43 40 73 0 50-37 80-85 80-47 0-85-30-85-78 0-33 21-61 52-75Z"
          fill="url(#bunny-highlight)"
        />
        <path
          d="M146 91c-6 21 3 66 20 105 5 10 12 5 9-6-5-29-12-75-23-98-2-4-4-4-6-1Z"
          fill="url(#ear-shine)"
        />
        <path
          d="M262 91c-13 11-25 42-33 68"
          fill="none"
          stroke="#94c4ff"
          strokeWidth="5"
          strokeLinecap="round"
          opacity=".5"
        />
        <path
          d="M146 273c5-16 18-28 34-33"
          stroke="#81baff"
          strokeWidth="5"
          fill="none"
          strokeLinecap="round"
          opacity=".5"
        />
        <ellipse cx="178" cy="279" rx="4.4" ry="6" fill="#f6faff" />
        <ellipse cx="225" cy="277" rx="4.4" ry="6" fill="#f6faff" />
        <path
          d="m197 293 5 3 5-3m-5 3v5"
          stroke="#d5e9ff"
          fill="none"
          strokeWidth="2.7"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M220 329c17-38 41-49 71-45-5 38-31 66-68 68-10 0-10-11-3-23Z"
          fill="url(#moon-shine)"
        />
        <ellipse
          cx="218"
          cy="334"
          rx="18"
          ry="13"
          transform="rotate(-30 218 334)"
          fill={colors[1]}
        />
        <path
          d="M299 187c2-14 8-22 23-25-15-3-21-11-23-25-3 14-9 22-24 25 15 3 21 11 24 25Z"
          fill={colors[0]}
        />
        <path
          d="M339 223c1-7 5-11 12-13-7-1-11-5-12-12-2 7-5 11-12 12 7 2 10 6 12 13Z"
          fill="#99bbf4"
        />
      </g>
    </svg>
  );
}
