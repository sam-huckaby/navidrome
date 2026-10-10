import React from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AudiobookCategoryButton from './AudiobookCategoryButton'

const saveCategory = vi.fn()
const push = vi.fn()
const access = { permissions: 'admin' }
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  useDataProvider: () => ({ setAudiobookCategory: saveCategory }),
  useNotify: () => vi.fn(),
  usePermissions: () => access,
}))
vi.mock('react-router-dom', async (importOriginal) => ({
  ...(await importOriginal()),
  useHistory: () => ({ push }),
}))

describe('AudiobookCategoryButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    access.permissions = 'admin'
    saveCategory.mockResolvedValue({ data: { id: 'al-1' } })
  })

  const show = (isAudiobook) =>
    render(
      <ThemeProvider theme={createTheme()}>
        <AudiobookCategoryButton album={{ id: 'al-1', isAudiobook }} />
      </ThemeProvider>,
    )

  it('marks an album and opens its audiobook player', async () => {
    show(false)
    fireEvent.click(screen.getByRole('button', { name: 'Mark as audiobook' }))
    await waitFor(() => expect(saveCategory).toHaveBeenCalledWith('al-1', true))
    expect(push).toHaveBeenCalledWith('/audiobooks/al-1')
  })

  it('can return a book to music browsing', async () => {
    show(true)
    fireEvent.click(screen.getByRole('button', { name: 'Mark as music' }))
    await waitFor(() =>
      expect(saveCategory).toHaveBeenCalledWith('al-1', false),
    )
    expect(push).toHaveBeenCalledWith('/album/al-1/show')
  })

  it('is not available to non-admins', () => {
    access.permissions = 'regular'
    show(false)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
