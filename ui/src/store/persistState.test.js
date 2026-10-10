import { afterEach, describe, expect, it } from 'vitest'
import { loadState } from './persistState'

describe('saved state migration', () => {
  afterEach(() => localStorage.clear())

  it('discards preferences from the removed theme picker without losing playback state', () => {
    localStorage.setItem(
      'state',
      JSON.stringify({ theme: 'DarkTheme', player: { volume: 0.5 } }),
    )
    expect(loadState()).toEqual({ player: { volume: 0.5 } })
  })
})
