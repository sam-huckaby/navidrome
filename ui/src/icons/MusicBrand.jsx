import React from 'react'
import { Link } from 'react-router-dom'

export const MusicMark = ({ large = false }) => (
  <span
    className={`music-brand-mark${large ? ' music-brand-mark--large' : ''}`}
    aria-hidden="true"
  >
    M
  </span>
)

const MusicBrand = () => (
  <Link className="music-brand" to="/" aria-label="Music: Server home">
    <MusicMark />
    <span className="music-brand-name">
      Music<span className="music-brand-suffix">: Server</span>
    </span>
  </Link>
)

export default MusicBrand
