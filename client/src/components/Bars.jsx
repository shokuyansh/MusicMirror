export default function Bars({ small = false, active = true }) {
  return (
    <span
      className={`equalizer ${small ? 'small' : ''} ${active ? 'moving' : ''}`}
      aria-hidden="true"
    >
      {Array.from({ length: small ? 4 : 19 }, (_, i) => (
        <i
          key={i}
          style={{
            '--bar-height': `${20 + ((i * 37 + 19) % 80)}%`,
            '--delay': `${(i % 7) * -0.17}s`,
          }}
        />
      ))}
    </span>
  )
}
