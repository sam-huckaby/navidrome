import { useEffect } from 'react'
import music from './music'

// There is one visual identity for Music: Server, including for users whose
// browser still contains a persisted preference for a removed theme.
const theme = {
  ...music,
  props: {
    MuiUseMediaQuery: { noSsr: true },
    MuiPopover: { disableScrollLock: true },
    MuiSwitch: { color: 'primary' },
  },
}

let themeUsers = 0

const useCurrentTheme = () => {
  useEffect(() => {
    document.body.style.backgroundColor = music.palette.background.default
    if (themeUsers++ === 0) {
      const style = document.createElement('style')
      style.id = 'music-player-style'
      style.textContent = music.player.stylesheet
      document.head.appendChild(style)
    }
    return () => {
      if (--themeUsers === 0) {
        document.getElementById('music-player-style')?.remove()
      }
    }
  }, [])
  return theme
}

export default useCurrentTheme
