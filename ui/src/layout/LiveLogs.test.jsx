import React from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import LiveLogs from './LiveLogs'

let permission = 'admin'
const streams = []
class FakeEventSource {
  constructor(url) {
    this.url = url
    this.handlers = {}
    this.close = vi.fn()
    streams.push(this)
  }
  addEventListener(name, handler) {
    this.handlers[name] = handler
  }
  emit(record) {
    this.handlers.log({ data: JSON.stringify(record) })
  }
}

vi.mock('react-admin', () => ({
  Title: () => null,
  usePermissions: () => ({ permissions: permission }),
  useTranslate: () => (key) => key,
}))
vi.mock('../utils', () => ({ baseUrl: (path) => `/music${path}` }))

beforeEach(() => {
  permission = 'admin'
  streams.length = 0
  vi.stubGlobal('EventSource', FakeEventSource)
  localStorage.setItem('token', 'session-token')
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  localStorage.removeItem('token')
})

describe('LiveLogs', () => {
  it('streams entries, pauses, filters and closes when leaving the page', () => {
    const { unmount } = render(<LiveLogs />)
    expect(streams[0].url).toContain('/music/api/logs/stream?jwt=session-token')
    act(() => {
      streams[0].emit({
        id: 1,
        time: '2026-01-01T00:00:00Z',
        level: 'info',
        message: 'started',
      })
    })
    expect(screen.getByText('started')).toBeTruthy()

    fireEvent.click(screen.getByText('logs.pause'))
    act(() => {
      streams[0].emit({
        id: 2,
        time: '2026-01-01T00:00:00Z',
        level: 'error',
        message: 'failed',
      })
    })
    expect(screen.queryByText('failed')).toBeNull()
    fireEvent.click(screen.getByText('logs.resume'))
    expect(screen.getByText('failed')).toBeTruthy()

    fireEvent.mouseDown(screen.getByRole('button', { name: 'logs.level' }))
    fireEvent.click(screen.getByRole('option', { name: 'logs.levels.error' }))
    expect(screen.queryByText('started')).toBeNull()
    expect(screen.getByText('failed')).toBeTruthy()

    fireEvent.click(screen.getByText('logs.clear'))
    expect(screen.queryByText('failed')).toBeNull()
    unmount()
    expect(streams[0].close).toHaveBeenCalled()
  })

  it('does not connect for non-admins', () => {
    permission = 'regular'
    const { container } = render(<LiveLogs />)
    expect(container.textContent).toBe('')
    expect(streams).toHaveLength(0)
  })

  it('reconnects from the last entry and handles a server restart', () => {
    vi.useFakeTimers()
    const { unmount } = render(<LiveLogs />)
    act(() => {
      streams[0].emit({
        id: 50,
        time: '2026-01-01T00:00:00Z',
        level: 'info',
        message: 'before restart',
      })
      streams[0].onerror()
    })
    expect(streams[0].close).toHaveBeenCalled()
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    expect(streams[1].url).toContain('since=50')
    act(() => {
      streams[1].emit({
        id: 1,
        time: '2026-01-01T00:00:00Z',
        level: 'info',
        message: 'after restart',
      })
    })
    expect(screen.queryByText('before restart')).toBeNull()
    expect(screen.getByText('after restart')).toBeTruthy()
    unmount()
    expect(streams[1].close).toHaveBeenCalled()
  })
})
