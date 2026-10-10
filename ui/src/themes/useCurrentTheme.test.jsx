import React from 'react'
import { renderHook } from '@testing-library/react-hooks'
import { render, screen } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@material-ui/core/styles'
import Switch from '@material-ui/core/Switch'
import useCurrentTheme from './useCurrentTheme'

describe('Music theme', () => {
  it('uses the same palette regardless of saved preferences', () => {
    const { result } = renderHook(() => useCurrentTheme())
    expect(result.current.themeName).toBe('Music')
    expect(result.current.palette.primary.main).toBe('#e6b85c')
    expect(document.body.style.backgroundColor).toBe('rgb(17, 17, 15)')
  })

  it('renders switches with the gold primary color', () => {
    const { result } = renderHook(() => useCurrentTheme())
    render(
      <ThemeProvider theme={createTheme(result.current)}>
        <Switch checked onChange={() => {}} />
      </ThemeProvider>,
    )
    expect(
      screen.getByRole('checkbox').closest('.MuiSwitch-switchBase').classList,
    ).toContain('MuiSwitch-colorPrimary')
  })
})
