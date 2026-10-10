import React from 'react'
import { render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { describe, expect, it, vi } from 'vitest'
import Menu from './Menu'

const resources = [
  { name: 'song', hasList: true, options: {} },
  { name: 'radio', hasList: true, options: {} },
]
vi.mock('react-admin', () => ({
  MenuItemLink: ({ primaryText }) => (
    <span data-testid="nav-link">{primaryText}</span>
  ),
  useTranslate: () => (key) =>
    ({ 'resources.song.name': 'Songs', 'resources.radio.name': 'Radios' })[
      key
    ] || key,
  usePermissions: () => ({ permissions: 'regular' }),
  getResources: () => resources,
}))
vi.mock('react-redux', () => ({
  useSelector: (selector) =>
    selector({ admin: { ui: { sidebarOpen: true } }, player: { queue: [] } }),
}))
vi.mock('./SubMenu', () => ({ default: () => null }))
vi.mock('./LibrarySelector', () => ({ default: () => null }))
vi.mock('../common/LibrarySelector', () => ({ default: () => null }))
vi.mock('./PlaylistsSubMenu', () => ({ default: () => null }))
vi.mock('../album/albumLists', () => ({ default: {} }))
vi.mock('../config', () => ({ default: { devSidebarPlaylists: false } }))

describe('Menu', () => {
  it('places Audiobooks between Songs and Radios', () => {
    render(
      <ThemeProvider theme={createTheme()}>
        <Menu />
      </ThemeProvider>,
    )
    expect(
      screen.getAllByTestId('nav-link').map((link) => link.textContent),
    ).toEqual(['Songs', 'Audiobooks', 'Radios'])
  })
})
