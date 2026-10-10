import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ThemeProvider, createTheme } from '@material-ui/core/styles'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import EditAlbumTags from './EditAlbumTags'

const mockPermissions = { value: 'admin' }
const mockGetAlbumTags = vi.fn()
const mockUpdateAlbumTags = vi.fn()
const mockClearAlbumTags = vi.fn()
const mockGetList = vi.fn()
const mockRefresh = vi.fn()
const mockNotify = vi.fn()
const mockDataProvider = {
  getAlbumTags: mockGetAlbumTags,
  updateAlbumTags: mockUpdateAlbumTags,
  clearAlbumTags: mockClearAlbumTags,
  getList: mockGetList,
}

vi.mock('react-admin', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useNotify: () => mockNotify,
    useDataProvider: () => mockDataProvider,
    useRefresh: () => mockRefresh,
    usePermissions: () => ({ permissions: mockPermissions.value }),
  }
})

describe('EditAlbumTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPermissions.value = 'admin'
    mockGetAlbumTags.mockResolvedValue({
      data: { tags: { genre: ['Rock'] }, overrides: {} },
    })
    mockGetList.mockResolvedValue({
      data: [{ tagName: 'genre', tagValue: 'Jazz' }],
    })
    mockUpdateAlbumTags.mockResolvedValue({ data: { id: 'al1' } })
    mockClearAlbumTags.mockResolvedValue({ data: { id: 'al1' } })
  })

  const renderEditor = () =>
    render(
      <ThemeProvider theme={createTheme()}>
        <EditAlbumTags record={{ id: 'al1', name: 'Album' }} />
      </ThemeProvider>,
    )

  it('is only available to admins', () => {
    mockPermissions.value = 'regular'
    renderEditor()
    expect(
      screen.queryByRole('button', { name: 'Edit album tags' }),
    ).not.toBeInTheDocument()
  })

  it('saves an album-only genre change', async () => {
    renderEditor()
    fireEvent.click(screen.getByRole('button', { name: 'Edit album tags' }))
    const input = await screen.findByRole('textbox', { name: 'Genres' })
    fireEvent.change(input, { target: { value: 'Jazz' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(mockUpdateAlbumTags).toHaveBeenCalledWith('al1', {
        genre: ['Rock', 'Jazz'],
      }),
    )
    expect(mockRefresh).toHaveBeenCalled()
  })

  it('offers Retry rather than spinning forever when loading fails', async () => {
    mockGetAlbumTags.mockRejectedValueOnce(new Error('Temporary error'))
    renderEditor()
    fireEvent.click(screen.getByRole('button', { name: 'Edit album tags' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load album tags',
    )
    expect(
      screen.queryByLabelText('Loading album tags'),
    ).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('textbox', { name: 'Genres' }),
    ).toBeInTheDocument()
  })
})
