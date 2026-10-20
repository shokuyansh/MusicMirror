import Icon from './Icon'

export default function Brand() {
  return (
    <div className="brand">
      <span className="brand-icon">
        <Icon name="headphones" size={23} />
      </span>
      <span>
        music<span className="brand-light">mirror</span>
        <span className="brand-dot">.</span>
      </span>
    </div>
  )
}
