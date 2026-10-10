import React from 'react'
import { render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { describe, expect, it, vi } from 'vitest'
import AudiobookLibrary from './AudiobookLibrary'

const mockedList = vi.fn()
vi.mock('../common', () => ({
  List: ({ children, ...props }) => {
    mockedList(props)
    return <div>{children}</div>
  },
}))
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  Title: () => null,
  useListContext: () => ({
    ids: ['al-book'],
    data: {
      'al-book': { id: 'al-book', name: 'The Book', albumArtist: 'Reader' },
    },
    loading: false,
  }),
}))
vi.mock('react-router-dom', async (importOriginal) => ({
  ...(await importOriginal()),
  Link: ({ to, children, ...rest }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))
vi.mock('../subsonic', () => ({ default: { getCoverArtUrl: () => '/cover' } }))

describe('AudiobookLibrary', () => {
  it('filters books and links covers to the dedicated player', () => {
    render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookLibrary />
      </ThemeProvider>,
    )
    expect(mockedList).toHaveBeenCalledWith(
      expect.objectContaining({
        resource: 'album',
        filter: { audiobook: true },
      }),
    )
    expect(screen.getByRole('link', { name: /The Book/ })).toHaveAttribute(
      'href',
      '/audiobooks/al-book',
    )
  })
})
