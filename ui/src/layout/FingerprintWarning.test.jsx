import React from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FingerprintWarning from './FingerprintWarning'

const mockStatus = vi.fn()
const dataProvider = { musicUploadStatus: mockStatus }
const access = { permissions: 'admin' }
vi.mock('react-admin', async (importOriginal) => ({
  ...(await importOriginal()),
  useDataProvider: () => dataProvider,
  usePermissions: () => access,
}))

describe('FingerprintWarning', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    access.permissions = 'admin'
  })

  const show = () =>
    render(
      <ThemeProvider theme={createTheme()}>
        <FingerprintWarning />
      </ThemeProvider>,
    )

  it('shows an accessible warning when fpcalc is missing', async () => {
    mockStatus.mockResolvedValue({
      data: { fpcalcInstalled: false, matchingConfigured: true },
    })
    show()
    expect(
      await screen.findByRole('button', {
        name: /fingerprinting library is not installed/i,
      }),
    ).toBeInTheDocument()
  })

  it('stays quiet when matching is available or the user is not an admin', async () => {
    mockStatus.mockResolvedValue({
      data: { fpcalcInstalled: true, matchingConfigured: true },
    })
    show()
    await waitFor(() => expect(mockStatus).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    access.permissions = 'regular'
    show()
    expect(mockStatus).toHaveBeenCalledTimes(1)
  })
})
