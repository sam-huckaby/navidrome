import React from 'react'
import { render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import AudiobookLibrary from './AudiobookLibrary'

const useListContext = vi.fn()

vi.mock('react-admin', () => ({
  Title: () => null,
  useListContext: () => useListContext(),
}))
vi.mock('../common', () => ({
  List: ({ basePath, children }) => (
    <div data-testid="audiobook-list" data-base-path={basePath}>
      {children}
    </div>
  ),
}))
vi.mock('../subsonic', () => ({
  default: { getCoverArtUrl: () => '' },
}))

describe('AudiobookLibrary', () => {
  it('renders while its list context is not initialized', () => {
    useListContext.mockReturnValue({ loading: true })

    render(
      <ThemeProvider theme={createTheme()}>
        <MemoryRouter>
          <AudiobookLibrary />
        </MemoryRouter>
      </ThemeProvider>,
    )

    expect(screen.getByRole('heading', { name: 'Audiobooks' })).toBeInTheDocument()
    expect(screen.getByTestId('audiobook-list')).toHaveAttribute(
      'data-base-path',
      '/album',
    )
  })

  it('shows the empty state once audiobooks finish loading', () => {
    useListContext.mockReturnValue({ ids: [], data: {}, loading: false })

    render(
      <ThemeProvider theme={createTheme()}>
        <MemoryRouter>
          <AudiobookLibrary />
        </MemoryRouter>
      </ThemeProvider>,
    )

    expect(screen.getByText(/No audiobooks yet/)).toBeInTheDocument()
  })
})
