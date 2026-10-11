import React, { useEffect, useRef, useState } from 'react'
import { Title, usePermissions, useTranslate } from 'react-admin'
import {
  Box,
  Button,
  Card,
  CardContent,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Typography,
  makeStyles,
} from '@material-ui/core'
import { baseUrl } from '../utils'

const MAX_ENTRIES = 500
const RECONNECT_DELAY = 3000
const levels = ['all', 'fatal', 'error', 'warning', 'info', 'debug', 'trace']

const useStyles = makeStyles((theme) => ({
  root: { margin: theme.spacing(2), marginBottom: 100 },
  toolbar: {
    display: 'flex',
    alignItems: 'center',
    flexWrap: 'wrap',
    gap: theme.spacing(2),
    marginBottom: theme.spacing(2),
  },
  level: { minWidth: 130 },
  output: {
    height: 'calc(100vh - 270px)',
    minHeight: 300,
    overflowY: 'auto',
    backgroundColor: theme.palette.background.default,
    padding: theme.spacing(1),
    fontFamily: 'monospace',
    fontSize: 13,
  },
  line: { whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', padding: '2px 0' },
  error: { color: theme.palette.error.main },
  warning: { color: theme.palette.warning.main },
  timestamp: { color: theme.palette.text.secondary },
}))

const LiveLogs = () => {
  const { permissions } = usePermissions()
  const translate = useTranslate()
  const classes = useStyles()
  const [entries, setEntries] = useState([])
  const [paused, setPaused] = useState(false)
  const [level, setLevel] = useState('all')
  const [status, setStatus] = useState('connecting')
  const entriesRef = useRef([])
  const pausedRef = useRef(false)
  const lastIdRef = useRef(null)
  const outputRef = useRef(null)

  useEffect(() => {
    if (permissions !== 'admin') return
    let stream
    let timer
    let stopped = false

    const connect = () => {
      let url = baseUrl('/api/logs/stream')
      const params = new URLSearchParams()
      const token = localStorage.getItem('token')
      if (token) params.set('jwt', token)
      if (lastIdRef.current !== null) params.set('since', lastIdRef.current)
      if (params.size) url += `?${params.toString()}`
      stream = new EventSource(url)
      stream.onopen = () => setStatus('live')
      stream.addEventListener('log', (event) => {
        try {
          const record = JSON.parse(event.data)
          // A smaller ID means the server restarted and began a new history.
          if (lastIdRef.current !== null && record.id <= lastIdRef.current) {
            entriesRef.current = []
          }
          lastIdRef.current = record.id
          entriesRef.current = [...entriesRef.current, record].slice(
            -MAX_ENTRIES,
          )
          if (!pausedRef.current) setEntries(entriesRef.current)
        } catch {
          // Ignore a malformed entry without losing the rest of the stream.
        }
      })
      stream.onerror = () => {
        stream.close()
        if (stopped) return
        setStatus('reconnecting')
        timer = setTimeout(connect, RECONNECT_DELAY)
      }
    }

    connect()
    return () => {
      stopped = true
      clearTimeout(timer)
      stream?.close()
    }
  }, [permissions])

  useEffect(() => {
    const output = outputRef.current
    if (
      output &&
      output.scrollHeight - output.scrollTop - output.clientHeight < 100
    ) {
      output.scrollTop = output.scrollHeight
    }
  }, [entries, level])

  if (permissions !== 'admin') return null

  const togglePause = () => {
    pausedRef.current = !pausedRef.current
    setPaused(pausedRef.current)
    if (!pausedRef.current) setEntries(entriesRef.current)
  }

  const clear = () => {
    entriesRef.current = []
    setEntries([])
  }

  return (
    <div className={classes.root}>
      <Title title={translate('logs.title')} />
      <Card>
        <CardContent>
          <Box className={classes.toolbar}>
            <Typography variant="h6">{translate('logs.title')}</Typography>
            <Typography role="status">{translate(`logs.${status}`)}</Typography>
            <Button onClick={togglePause} variant="outlined">
              {translate(paused ? 'logs.resume' : 'logs.pause')}
            </Button>
            <Button onClick={clear} variant="outlined">
              {translate('logs.clear')}
            </Button>
            <FormControl className={classes.level}>
              <InputLabel id="log-level-label">
                {translate('logs.level')}
              </InputLabel>
              <Select
                labelId="log-level-label"
                value={level}
                onChange={(event) => setLevel(event.target.value)}
              >
                {levels.map((choice) => (
                  <MenuItem key={choice} value={choice}>
                    {translate(`logs.levels.${choice}`)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>
          <div
            ref={outputRef}
            className={classes.output}
            role="log"
            aria-live="off"
          >
            {entries
              .filter((entry) => level === 'all' || entry.level === level)
              .map((entry) => (
                <div key={entry.id} className={classes.line}>
                  <span className={classes.timestamp}>
                    {new Date(entry.time).toLocaleString()}
                  </span>{' '}
                  <span className={classes[entry.level] || ''}>
                    [{entry.level.toUpperCase()}]
                  </span>{' '}
                  {entry.message}
                </div>
              ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export default LiveLogs
