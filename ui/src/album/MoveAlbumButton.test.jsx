import React from 'react'
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider, createTheme } from '@material-ui/core/styles'
import MoveAlbumButton from './MoveAlbumButton'

const provider = { getList: vi.fn(), moveAlbum: vi.fn(), getAlbumMove: vi.fn() }
const dispatch = vi.fn()
const notify = vi.fn()
const replace = vi.fn()
const refresh = vi.fn()
let permissions = 'admin'
vi.mock('react-admin', () => ({
  useDataProvider: () => provider,
  usePermissions: () => ({ permissions }),
  useNotify: () => notify,
  useRefresh: () => refresh,
}))
vi.mock('react-redux', () => ({ useDispatch: () => dispatch }))
vi.mock('react-router-dom', () => ({ useHistory: () => ({ replace }) }))

const album = {
  id: 'old-album',
  name: 'A Book',
  libraryId: 1,
  isAudiobook: true,
}
const show = (props = {}) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MoveAlbumButton album={album} {...props} />
    </ThemeProvider>,
  )

describe('MoveAlbumButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    permissions = 'admin'
    provider.getList.mockResolvedValue({
      data: [
        { id: 1, name: 'Family', path: '/family' },
        { id: 2, name: 'Adults', path: '/adults' },
      ],
    })
    provider.moveAlbum.mockResolvedValue({
      data: { id: 'job-1', state: 'moving' },
    })
    provider.getAlbumMove.mockResolvedValue({
      data: { id: 'job-1', state: 'complete', newAlbumId: 'new-album' },
    })
  })

  it('is hidden from non-admin users', () => {
    permissions = 'regular'
    show()
    expect(
      screen.queryByRole('button', { name: /Move to library/ }),
    ).not.toBeInTheDocument()
    expect(provider.getList).not.toHaveBeenCalled()
  })

  it('moves to a different library, polls server status, and opens the new audiobook', async () => {
    const onStarting = vi.fn()
    show({ onStarting })
    fireEvent.click(screen.getByRole('button', { name: /Move to library/ }))
    await waitFor(() => expect(provider.getList).toHaveBeenCalled())
    fireEvent.mouseDown(
      screen.getByRole('button', { name: /Destination library/ }),
    )
    const list = within(await screen.findByRole('listbox'))
    expect(
      list.queryByRole('option', { name: 'Family' }),
    ).not.toBeInTheDocument()
    fireEvent.click(list.getByRole('option', { name: 'Adults' }))
    fireEvent.click(screen.getByRole('button', { name: 'Move', exact: true }))
    await waitFor(() =>
      expect(provider.moveAlbum).toHaveBeenCalledWith('old-album', 2),
    )
    expect(onStarting).toHaveBeenCalled()
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith('/audiobooks/new-album'),
    )
    expect(localStorage.getItem('album-move:old-album')).toBeNull()
  })

  it('resumes status polling after reopening a page with an active move', async () => {
    localStorage.setItem('album-move:old-album', 'job-1')
    show()
    await waitFor(() =>
      expect(provider.getAlbumMove).toHaveBeenCalledWith('job-1'),
    )
    await waitFor(() =>
      expect(replace).toHaveBeenCalledWith('/audiobooks/new-album'),
    )
    expect(provider.moveAlbum).not.toHaveBeenCalled()
  })

  it('shows a rejected move reason without polling or navigating', async () => {
    provider.moveAlbum.mockRejectedValue(
      new Error('album folder is shared with another album'),
    )
    show()
    fireEvent.click(screen.getByRole('button', { name: /Move to library/ }))
    await waitFor(() => expect(provider.getList).toHaveBeenCalled())
    fireEvent.mouseDown(
      screen.getByRole('button', { name: /Destination library/ }),
    )
    fireEvent.click(
      within(await screen.findByRole('listbox')).getByRole('option', {
        name: 'Adults',
      }),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Move', exact: true }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'album folder is shared with another album',
    )
    expect(provider.getAlbumMove).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })
})
