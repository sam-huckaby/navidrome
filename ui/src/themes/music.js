// The palette and typography are shared with Music: Client.
import stylesheet from './music.css'
export const colors = {
  background: '#11110f',
  sidebar: '#141411',
  surface: '#191916',
  raised: '#22221e',
  ink: '#f3eee4',
  muted: '#aaa79f',
  quiet: '#77756f',
  gold: '#e6b85c',
  goldDark: '#bd8537',
  line: 'rgba(255, 255, 255, 0.09)',
}

const music = {
  themeName: 'Music',
  palette: {
    type: 'dark',
    primary: {
      main: colors.gold,
      light: '#f2c66e',
      dark: colors.goldDark,
      contrastText: '#17140e',
    },
    secondary: { main: colors.gold },
    background: { default: colors.background, paper: colors.surface },
    text: {
      primary: colors.ink,
      secondary: colors.muted,
      disabled: colors.quiet,
    },
    divider: colors.line,
    error: { main: '#e88873' },
  },
  typography: {
    fontFamily: '"DM Sans", system-ui, sans-serif',
    h1: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    h2: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    h3: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    h4: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    h5: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    h6: { fontFamily: '"Newsreader", Georgia, serif', fontWeight: 600 },
    button: { textTransform: 'none', fontWeight: 700 },
  },
  shape: { borderRadius: 8 },
  sidebar: { width: 224, closedWidth: 56 },
  overrides: {
    RaLayout: {
      appFrame: { marginTop: '64px !important' },
      content: {
        padding: '20px clamp(16px, 3vw, 44px) 110px !important',
        minWidth: 0,
        background:
          'radial-gradient(circle at 85% 0, rgba(113,88,45,.12), transparent 35%), #11110f',
      },
    },
    RaSidebar: {
      // The sidebar's fixed inner panel handles scrolling; the docked wrapper
      // must not force a full viewport of height below the app bar.
      root: { height: 'auto' },
      fixed: {
        height: 'calc(100vh - 64px)',
        backgroundColor: colors.sidebar,
        borderRight: `1px solid ${colors.line}`,
      },
      drawerPaper: {
        backgroundColor: colors.sidebar,
        borderRight: `1px solid ${colors.line}`,
      },
    },
    MuiCssBaseline: {
      '@global': { body: { backgroundColor: colors.background } },
    },
    MuiAppBar: {
      colorSecondary: { backgroundColor: colors.background, color: colors.ink },
      root: {
        backgroundColor: 'rgba(17, 17, 15, 0.96)',
        borderBottom: `1px solid ${colors.line}`,
        boxShadow: 'none',
        '& .MuiToolbar-root': { minHeight: 64 },
      },
    },
    MuiDrawer: {
      paper: {
        backgroundColor: colors.sidebar,
        borderRight: `1px solid ${colors.line}`,
      },
    },
    MuiPaper: { rounded: { borderRadius: 8 } },
    MuiCard: {
      root: {
        backgroundColor: colors.surface,
        border: `1px solid ${colors.line}`,
        boxShadow: 'none',
      },
    },
    MuiButton: {
      root: { borderRadius: 6, minHeight: 40 },
      contained: { boxShadow: 'none', '&:hover': { boxShadow: 'none' } },
    },
    MuiListItem: {
      button: {
        borderRadius: 8,
        '&:hover': { backgroundColor: colors.raised },
      },
    },
    MuiListItemIcon: { root: { color: colors.muted, minWidth: 38 } },
    MuiTableCell: {
      root: { borderBottom: `1px solid ${colors.line}` },
      head: { color: colors.muted, fontWeight: 700 },
    },
    MuiTableRow: {
      hover: { '&:hover': { backgroundColor: `${colors.raised} !important` } },
    },
    MuiOutlinedInput: {
      root: { backgroundColor: colors.surface },
      notchedOutline: { borderColor: colors.line },
    },
    MuiInput: { underline: { '&:before': { borderBottomColor: colors.line } } },
    MuiDialog: {
      paper: {
        backgroundColor: colors.surface,
        border: `1px solid ${colors.line}`,
      },
    },
    MuiPopover: {
      paper: {
        backgroundColor: colors.raised,
        border: `1px solid ${colors.line}`,
      },
    },
    MuiTooltip: {
      tooltip: { backgroundColor: colors.raised, color: colors.ink },
    },
    NDAlbumGridView: {
      link: {
        borderRadius: 5,
        overflow: 'hidden',
        boxShadow: '0 10px 28px rgba(0,0,0,.28)',
      },
      albumName: { color: colors.ink, fontWeight: 600 },
      albumSubtitle: { color: colors.muted },
      albumVersion: { color: colors.muted },
    },
    NDMobileArtistDetails: {
      bgContainer: {
        background: 'linear-gradient(to bottom, #2e2920, #11110f) !important',
      },
    },
  },
  player: { theme: 'dark', stylesheet },
}

export default music
