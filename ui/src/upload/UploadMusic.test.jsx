import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UploadMusic from './UploadMusic'

const mockStartScan = vi.fn()
const provider = {
  getList: vi.fn(),
  musicUploadStatus: vi.fn(),
  createMusicUpload: vi.fn(),
  sendMusicUploadFile: vi.fn(),
  previewMusicUpload: vi.fn(),
  matchMusicUploadTrack: vi.fn(),
  importMusicUpload: vi.fn(),
  cancelMusicUpload: vi.fn(),
}
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  Title: () => null,
  useDataProvider: () => provider,
  usePermissions: () => ({ permissions: 'admin' }),
}))
vi.mock('../subsonic', () => ({
  default: { startScan: (...args) => mockStartScan(...args) },
}))

const selectLibrary = async (name = 'Music') => {
  fireEvent.mouseDown(
    screen.getByRole('button', { name: /^Destination library/ }),
  )
  fireEvent.click(await screen.findByRole('option', { name }))
}

describe('UploadMusic', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    provider.getList.mockResolvedValue({ data: [{ id: 1, name: 'Music' }] })
    provider.musicUploadStatus.mockResolvedValue({
      data: { fpcalcInstalled: false, matchingConfigured: false },
    })
    provider.createMusicUpload.mockResolvedValue({ data: { id: 'session-1' } })
    provider.sendMusicUploadFile.mockResolvedValue({ data: {} })
    provider.previewMusicUpload.mockResolvedValue({
      data: {
        tracks: [
          {
            path: 'Album/song.mp3',
            title: 'Song',
            artist: 'Artist',
            album: 'Album',
            albumArtist: 'Artist',
            needsMatch: false,
          },
        ],
        files: ['Album/song.mp3'],
      },
    })
    provider.importMusicUpload.mockResolvedValue({
      data: {
        imported: ['Album/song.mp3'],
        skipped: [],
        unmatched: [],
        libraryId: 1,
      },
    })
    mockStartScan.mockResolvedValue({})
    provider.matchMusicUploadTrack.mockResolvedValue({
      data: { candidates: [] },
    })
  })

  it('uploads a selected folder and scans the chosen library when fpcalc is missing', async () => {
    provider.getList.mockResolvedValue({
      data: [
        { id: 1, name: 'Music' },
        { id: 2, name: 'Audiobooks' },
      ],
    })
    provider.importMusicUpload.mockResolvedValue({
      data: {
        imported: ['Album/song.mp3'],
        skipped: [],
        unmatched: [],
        libraryId: 2,
      },
    })
    render(
      <ThemeProvider theme={createTheme()}>
        <UploadMusic />
      </ThemeProvider>,
    )
    const file = new File(['music'], 'song.mp3', { type: 'audio/mpeg' })
    Object.defineProperty(file, 'webkitRelativePath', {
      value: 'Album/song.mp3',
    })
    fireEvent.change(screen.getByLabelText('Choose a music folder'), {
      target: { files: [file] },
    })
    expect(screen.getByText('Select library')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Upload folder' })).toBeDisabled()
    await selectLibrary('Audiobooks')
    fireEvent.click(
      await screen.findByRole('button', { name: 'Upload folder' }),
    )
    expect(provider.createMusicUpload).toHaveBeenCalledWith(2)
    expect(await screen.findByText('Review metadata')).toBeInTheDocument()
    expect(provider.sendMusicUploadFile).toHaveBeenCalledWith('session-1', file)
    fireEvent.click(screen.getByRole('button', { name: 'Import music' }))
    expect(await screen.findByText('Import complete')).toBeInTheDocument()
    await waitFor(() =>
      expect(mockStartScan).toHaveBeenCalledWith({ target: ['2:'] }),
    )
    // The server removes completed sessions, so cleanup may return 404.
    provider.cancelMusicUpload.mockRejectedValue(new Error('Not found'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Upload another folder' }),
    )
    expect(
      await screen.findByLabelText('Choose a music folder'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Import complete')).not.toBeInTheDocument()
    expect(screen.getByText('Select library')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Upload folder' })).toBeDisabled()
  })

  it('automatically fills missing metadata from a confident matching recording', async () => {
    provider.musicUploadStatus.mockResolvedValue({
      data: { fpcalcInstalled: true, matchingConfigured: true },
    })
    provider.previewMusicUpload.mockResolvedValue({
      data: {
        tracks: [
          {
            path: 'Album/song.mp3',
            title: '',
            artist: 'Artist',
            album: 'Album',
            albumArtist: '',
            needsMatch: true,
          },
        ],
        files: ['Album/song.mp3'],
      },
    })
    provider.matchMusicUploadTrack.mockResolvedValue({
      data: {
        candidates: [
          {
            title: 'Matched song',
            artist: 'Artist',
            album: 'Album',
            score: 0.99,
          },
        ],
      },
    })
    render(
      <ThemeProvider theme={createTheme()}>
        <UploadMusic />
      </ThemeProvider>,
    )
    const file = new File(['music'], 'song.mp3', { type: 'audio/mpeg' })
    Object.defineProperty(file, 'webkitRelativePath', {
      value: 'Album/song.mp3',
    })
    fireEvent.change(screen.getByLabelText('Choose a music folder'), {
      target: { files: [file] },
    })
    await selectLibrary()
    await screen.findByRole('button', { name: 'Upload folder' })
    fireEvent.click(screen.getByRole('button', { name: 'Upload folder' }))
    expect(await screen.findByRole('textbox', { name: 'Title' })).toHaveValue(
      'Matched song',
    )
    expect(provider.matchMusicUploadTrack).toHaveBeenCalledWith(
      'session-1',
      'Album/song.mp3',
    )
  })

  it('marks only the selected album as an audiobook during upload review', async () => {
    provider.previewMusicUpload.mockResolvedValue({
      data: {
        tracks: [
          {
            path: 'First/a.mp3',
            title: 'One',
            artist: 'Reader',
            album: 'First',
            albumArtist: 'Reader',
            needsMatch: false,
          },
          {
            path: 'Second/b.mp3',
            title: 'Two',
            artist: 'Musician',
            album: 'Second',
            albumArtist: 'Musician',
            needsMatch: false,
          },
        ],
        files: ['First/a.mp3', 'Second/b.mp3'],
      },
    })
    render(
      <ThemeProvider theme={createTheme()}>
        <UploadMusic />
      </ThemeProvider>,
    )
    const a = new File(['one'], 'a.mp3')
    const b = new File(['two'], 'b.mp3')
    Object.defineProperty(a, 'webkitRelativePath', { value: 'First/a.mp3' })
    Object.defineProperty(b, 'webkitRelativePath', { value: 'Second/b.mp3' })
    fireEvent.change(screen.getByLabelText('Choose a music folder'), {
      target: { files: [a, b] },
    })
    await selectLibrary()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Upload folder' }),
    )
    const checkbox = await screen.findByRole('checkbox', {
      name: 'First — Audiobook',
    })
    fireEvent.click(checkbox)
    fireEvent.click(screen.getByRole('button', { name: 'Import music' }))
    await waitFor(() => expect(provider.importMusicUpload).toHaveBeenCalled())
    const sent = provider.importMusicUpload.mock.calls[0][1]
    expect(sent[0].isAudiobook).toBe(true)
    expect(sent[1].isAudiobook).toBeFalsy()
  })
})
