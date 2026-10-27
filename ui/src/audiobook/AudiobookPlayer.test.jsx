import React from 'react'
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AudiobookPlayer from './AudiobookPlayer'

const provider = {
  getOne: vi.fn(),
  getList: vi.fn(),
  getAudiobookProgress: vi.fn(),
  saveAudiobookProgress: vi.fn(),
  deleteAudiobook: vi.fn(),
}
const dispatch = vi.fn()
const push = vi.fn()
const refresh = vi.fn()
let permissions = 'regular'
const { streamUrl, startScan } = vi.hoisted(() => ({
  streamUrl: vi.fn((id) => `/stream/${id}?_=${Date.now()}`),
  startScan: vi.fn(),
}))
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  Title: () => null,
  useDataProvider: () => provider,
  useNotify: () => vi.fn(),
  usePermissions: () => ({ permissions }),
  useRefresh: () => refresh,
}))
vi.mock('react-router-dom', async (importOriginal) => ({
  ...(await importOriginal()),
  useParams: () => ({ id: 'al-book' }),
  useHistory: () => ({ push }),
  Link: ({ children }) => <span>{children}</span>,
}))
vi.mock('react-redux', () => ({
  useDispatch: () => dispatch,
  useSelector: (selector) => selector({ player: { queue: [] } }),
}))
vi.mock('../subsonic', () => ({
  default: {
    streamUrl,
    getCoverArtUrl: () => '/cover',
    startScan,
  },
}))

describe('AudiobookPlayer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permissions = 'regular'
    provider.getOne.mockResolvedValue({
      data: {
        id: 'al-book',
        name: 'The Book',
        albumArtist: 'Reader',
        isAudiobook: true,
        libraryId: 1,
      },
    })
    provider.getList.mockResolvedValue({
      data: [
        {
          id: 'chapter-1',
          title: 'Chapter 1',
          discNumber: 1,
          trackNumber: 1,
          duration: 100,
        },
        {
          id: 'chapter-2',
          title: 'Chapter 2',
          discNumber: 1,
          trackNumber: 2,
          duration: 180,
        },
      ],
    })
    provider.getAudiobookProgress.mockResolvedValue({
      data: { trackId: 'chapter-2', positionMs: 30000, completed: false },
    })
    provider.saveAudiobookProgress.mockResolvedValue({
      data: { id: 'al-book' },
    })
    provider.deleteAudiobook.mockResolvedValue({ data: { id: 'al-book' } })
    startScan.mockResolvedValue({})
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue()
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  })

  it('restores the saved chapter and seek position and persists new progress', async () => {
    const { container } = render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookPlayer />
      </ThemeProvider>,
    )
    expect(
      await screen.findByRole('heading', { name: 'Chapter 2' }),
    ).toBeInTheDocument()
    const audio = container.querySelector('audio')
    Object.defineProperty(audio, 'duration', { value: 180, configurable: true })
    fireEvent.loadedMetadata(audio)
    expect(audio.currentTime).toBe(30)
    fireEvent.click(screen.getByRole('button', { name: 'Resume' }))
    expect(dispatch).toHaveBeenCalled()
    Object.defineProperty(audio, 'paused', { value: false, configurable: true })
    audio.currentTime = 45
    fireEvent.timeUpdate(audio)
    await waitFor(() =>
      expect(provider.saveAudiobookProgress).toHaveBeenCalledWith('al-book', {
        trackId: 'chapter-2',
        positionMs: 45000,
        completed: false,
      }),
    )
  })

  it('keeps the stream URL stable during playback updates and changes it for a new chapter', async () => {
    const { container } = render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookPlayer />
      </ThemeProvider>,
    )
    await screen.findByRole('heading', { name: 'Chapter 2' })
    const audio = container.querySelector('audio')
    const originalSrc = audio.src
    const initialCalls = streamUrl.mock.calls.length

    fireEvent.loadedMetadata(audio)
    fireEvent.click(screen.getByRole('button', { name: 'Resume' }))
    fireEvent.play(audio)
    audio.currentTime = 35
    fireEvent.timeUpdate(audio)
    fireEvent.pause(audio)

    expect(audio.src).toBe(originalSrc)
    expect(streamUrl).toHaveBeenCalledTimes(initialCalls)

    fireEvent.click(screen.getByRole('button', { name: 'Previous chapter' }))
    await screen.findByRole('heading', { name: 'Chapter 1' })
    expect(container.querySelector('audio').src).not.toBe(originalSrc)
    expect(streamUrl).toHaveBeenCalledTimes(initialCalls + 1)
  })

  it('marks an audiobook complete after the final chapter ends', async () => {
    const { container } = render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookPlayer />
      </ThemeProvider>,
    )
    await screen.findByRole('heading', { name: 'Chapter 2' })
    const audio = container.querySelector('audio')
    Object.defineProperty(audio, 'duration', { value: 180, configurable: true })
    fireEvent.loadedMetadata(audio)
    fireEvent.ended(audio)
    await waitFor(() =>
      expect(provider.saveAudiobookProgress).toHaveBeenCalledWith('al-book', {
        trackId: 'chapter-2',
        positionMs: 180000,
        completed: true,
      }),
    )
    expect(screen.getByText(/Finished/)).toBeInTheDocument()
  })

  it('only lets admins confirm permanent deletion and returns to the audiobook library', async () => {
    const { unmount } = render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookPlayer />
      </ThemeProvider>,
    )
    await screen.findByRole('heading', { name: 'Chapter 2' })
    expect(screen.queryByRole('button', { name: 'Delete book' })).toBeNull()
    unmount()
    await waitFor(() =>
      expect(provider.saveAudiobookProgress).toHaveBeenCalled(),
    )
    provider.saveAudiobookProgress.mockClear()

    permissions = 'admin'
    const view = render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookPlayer />
      </ThemeProvider>,
    )
    await screen.findByRole('heading', { name: 'Chapter 2' })
    fireEvent.click(screen.getByRole('button', { name: 'Delete book' }))
    expect(screen.getByText(/Permanently delete The Book/)).toBeInTheDocument()
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Cancel',
      }),
    )
    expect(provider.deleteAudiobook).not.toHaveBeenCalled()
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Delete book' }))
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Delete book',
      }),
    )
    await waitFor(() => expect(push).toHaveBeenCalledWith('/audiobooks'))
    expect(provider.deleteAudiobook).toHaveBeenCalledWith('al-book')
    expect(refresh).toHaveBeenCalled()
    expect(startScan).toHaveBeenCalledWith({ target: ['1:'] })
    view.unmount()
    expect(provider.saveAudiobookProgress).not.toHaveBeenCalled()
  })
})
