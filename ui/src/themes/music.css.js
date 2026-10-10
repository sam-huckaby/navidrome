// The player renders outside the React Admin layout. Its styles must also be
// applied to the queue and full-screen player supplied by the player library.
const stylesheet = `
.react-jinke-music-player-main { font-family: 'DM Sans', system-ui, sans-serif; color: #f3eee4; }
.react-jinke-music-player-main .music-player-panel {
  color: #f3eee4 !important;
  background: rgba(35, 34, 30, .97) !important;
  border-top: 1px solid rgba(255,255,255,.14);
  box-shadow: 0 -15px 55px rgba(0,0,0,.36);
  backdrop-filter: blur(18px);
}
.react-jinke-music-player-main .music-player-panel .panel-content div.img-rotate,
.react-jinke-music-player-main .music-player-panel .panel-content img {
  border-radius: 5px !important;
}
.react-jinke-music-player-main svg:hover,
.react-jinke-music-player-main svg:active,
.react-jinke-music-player-main .audio-item.playing svg,
.react-jinke-music-player-main .audio-item.playing .player-singer,
.audio-lists-panel-content .audio-item.playing,
.audio-lists-panel-content .audio-item.playing svg { color: #e6b85c !important; }
.react-jinke-music-player-main .music-player-panel .rc-slider-track,
.react-jinke-music-player-main .music-player-panel .rc-slider-handle,
.react-jinke-music-player-main .rc-slider-track,
.react-jinke-music-player-main .rc-slider-handle { background: #e6b85c !important; border-color: #e6b85c !important; }
.react-jinke-music-player-main .rc-slider-rail { background: #4b4942 !important; }
.react-jinke-music-player-main .rc-slider-handle:focus-visible { outline: 2px solid #e6b85c; outline-offset: 3px; }
.react-jinke-music-player-main ::-webkit-scrollbar-thumb { background: #4d483d; }
.react-jinke-music-player-main .audio-lists-panel,
.react-jinke-music-player-main .audio-lists-panel-content,
.react-jinke-music-player-main .react-jinke-music-player-mobile { background: #191916 !important; color: #f3eee4 !important; }
.react-jinke-music-player-main .audio-item:hover { background: #22221e !important; }
`

export default stylesheet
