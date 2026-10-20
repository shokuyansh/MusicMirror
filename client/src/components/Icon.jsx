const paths = {
  headphones:
    'M3 14v-3a9 9 0 0 1 18 0v3 M3 13h3v8H4a1 1 0 0 1-1-1v-7 M21 13h-3v8h2a1 1 0 0 0 1-1v-7',
  music:
    'M9 18V5l12-2v13 M9 8l12-2 M9 18a3 3 0 1 1-3-3 3 3 0 0 1 3 3 M21 16a3 3 0 1 1-3-3 3 3 0 0 1 3 3',
  upload: 'M12 16V3 M7 8l5-5 5 5 M4 16v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4',
  copy: 'M9 9h12v12H9z M15 9V3H3v12h6',
  check: 'M5 12l4 4L19 6',
  play: 'M8 5l11 7-11 7z',
  pause: 'M8 5v14 M16 5v14',
  previous: 'M5 5v14 M19 5L7 12l12 7z',
  next: 'M19 5v14 M5 5l12 7-12 7z',
  arrow: 'M5 12h14 M13 6l6 6-6 6',
  leave: 'M9 4H4v16h5 M10 12h11 M16 7l5 5-5 5',
  x: 'M6 6l12 12 M18 6L6 18',
  volume: 'M11 5L5 9H2v6h3l6 4z M15 8a6 6 0 0 1 0 8 M18 5a10 10 0 0 1 0 14',
  mute: 'M11 5L5 9H2v6h3l6 4z M16 9l6 6 M22 9l-6 6',
  link: 'M10 13a5 5 0 0 0 7 0l4-4a5 5 0 0 0-7-7l-3 3 M14 11a5 5 0 0 0-7 0l-4 4a5 5 0 0 0 7 7l3-3',
  users:
    'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M22 21v-2a4 4 0 0 0-3-4 M9 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M17 3a4 4 0 0 1 0 8',
  refresh:
    'M20 7a8 8 0 0 0-14-2L3 8 M3 3v5h5 M4 17a8 8 0 0 0 14 2l3-3 M21 21v-5h-5',
}

export default function Icon({ name, size = 20, ...props }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={name === 'play' ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      <path d={paths[name] || paths.music} />
    </svg>
  )
}
