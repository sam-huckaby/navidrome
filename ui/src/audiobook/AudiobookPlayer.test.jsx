import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AudiobookPlayer from './AudiobookPlayer'

const provider = {
  getOne: vi.fn(),
  getList: vi.fn(),
  getAudiobookProgress: vi.fn(),
  saveAudiobookProgress: vi.fn(),
}
const dispatch = vi.fn()
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  Title: () => null,
  useDataProvider: () => provider,
  useNotify: () => vi.fn(),
  usePermissions: () => ({ permissions: 'regular' }),
}))
vi.mock('react-router-dom', async (importOriginal) => ({
  ...(await importOriginal()),
  useParams: () => ({ id: 'al-book' }),
  Link: ({ children }) => <span>{children}</span>,
}))
vi.mock('react-redux', () => ({
  useDispatch: () => dispatch,
  useSelector: (selector) => selector({ player: { queue: [] } }),
}))
vi.mock('../subsonic', () => ({
  default: {
    streamUrl: (id) => `/stream/${id}`,
    getCoverArtUrl: () => '/cover',
  },
}))

describe('AudiobookPlayer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    provider.getOne.mockResolvedValue({
      data: {
        id: 'al-book',
        name: 'The Book',
        albumArtist: 'Reader',
        isAudiobook: true,
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
})
