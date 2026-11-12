import React, { useEffect, useState } from 'react'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  LinearProgress,
  MenuItem,
  TextField,
  Typography,
} from '@material-ui/core'
import {
  useDataProvider,
  useNotify,
  usePermissions,
  useRefresh,
} from 'react-admin'
import { useHistory } from 'react-router-dom'
import { useDispatch } from 'react-redux'
import { clearQueue, setSelectedLibraries } from '../actions'

const MoveAlbumButton = ({ album, onStarting }) => {
  const { permissions } = usePermissions()
  const provider = useDataProvider()
  const notify = useNotify()
  const refresh = useRefresh()
  const history = useHistory()
  const dispatch = useDispatch()
  const [open, setOpen] = useState(false)
  const [libraries, setLibraries] = useState([])
  const [destination, setDestination] = useState('')
  const [job, setJob] = useState(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const storageKey = `album-move:${album?.id}`

  useEffect(() => {
    if (permissions !== 'admin' || !album?.id) return
    const saved = localStorage.getItem(storageKey)
    if (saved) {
      setJob({ id: saved, state: 'moving' })
      setOpen(true)
      setBusy(true)
    }
  }, [permissions, album?.id, storageKey])

  useEffect(() => {
    if (!job?.id || !busy) return
    let active = true
    let timer
    const poll = async () => {
      try {
        const { data } = await provider.getAlbumMove(job.id)
        if (!active) return
        setError('')
        setJob(data)
        if (data.state === 'complete' || data.state === 'failed') {
          setBusy(false)
          localStorage.removeItem(storageKey)
          if (data.state === 'failed')
            setError(data.error || 'Album move failed.')
          else {
            if (data.warning) notify(data.warning, 'warning')
            else notify('Album moved to the destination library.')
            dispatch(setSelectedLibraries([]))
            refresh()
            setOpen(false)
            history.replace(
              album.isAudiobook
                ? `/audiobooks/${data.newAlbumId}`
                : `/album/${data.newAlbumId}/show`,
            )
          }
          return
        }
        timer = setTimeout(poll, 1000)
      } catch (e) {
        if (!active) return
        setError(`${e.message || 'Could not check move status.'} Retrying…`)
        timer = setTimeout(poll, 3000)
      }
    }
    poll()
    return () => {
      active = false
      clearTimeout(timer)
    }
  }, [
    job?.id,
    busy,
    provider,
    storageKey,
    album.isAudiobook,
    history,
    refresh,
    notify,
    dispatch,
  ])

  if (permissions !== 'admin' || !album?.id) return null

  const show = async () => {
    setOpen(true)
    setError('')
    try {
      const { data } = await provider.getList('library', {
        pagination: { page: 1, perPage: 1000 },
        sort: { field: 'name', order: 'ASC' },
        filter: {},
      })
      setLibraries(data)
    } catch {
      setError('Could not load destination libraries.')
    }
  }
  const move = async () => {
    setError('')
    setBusy(true)
    try {
      await onStarting?.()
      dispatch(clearQueue())
      const { data } = await provider.moveAlbum(album.id, Number(destination))
      localStorage.setItem(storageKey, data.id)
      setJob(data)
    } catch (e) {
      setBusy(false)
      setError(e.message || 'Could not move album.')
    }
  }
  const source = libraries.find(
    (lib) => Number(lib.id) === Number(album.libraryId),
  )
  const target = libraries.find((lib) => String(lib.id) === String(destination))
  return (
    <>
      <Button size="small" disabled={album.missing} onClick={show}>
        Move to library…
      </Button>
      <Dialog
        open={open}
        onClose={() => {
          if (!busy) setOpen(false)
        }}
        fullWidth
        maxWidth="sm"
      >
        <DialogTitle>Move {album.name}</DialogTitle>
        <DialogContent>
          <Typography gutterBottom>
            Current library:{' '}
            {source?.name || album.libraryName || album.libraryId}
          </Typography>
          <Typography gutterBottom>
            The entire dedicated album folder, including artwork and chapters,
            will move with its relative paths preserved.
          </Typography>
          <TextField
            id={`move-library-${album.id}`}
            select
            fullWidth
            label="Destination library"
            value={destination}
            disabled={busy}
            onChange={(event) => setDestination(event.target.value)}
          >
            {libraries
              .filter((lib) => Number(lib.id) !== Number(album.libraryId))
              .map((lib) => (
                <MenuItem key={lib.id} value={String(lib.id)}>
                  {lib.name}
                </MenuItem>
              ))}
          </TextField>
          {target && (
            <Typography variant="body2">
              Destination root: {target.path}
            </Typography>
          )}
          {busy && (
            <>
              <LinearProgress />
              <Typography role="status">
                {job?.state === 'updating'
                  ? 'Updating catalog…'
                  : job?.state === 'scanning'
                    ? 'Scanning affected libraries…'
                    : 'Moving and verifying files…'}{' '}
                The move continues if you leave this page.
              </Typography>
            </>
          )}
          {job?.folder && (
            <Typography variant="body2">Album folder: {job.folder}</Typography>
          )}
          {error && (
            <Typography role="alert" color="error">
              {error}
            </Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button disabled={busy} onClick={() => setOpen(false)}>
            Close
          </Button>
          <Button
            color="primary"
            disabled={busy || !destination}
            onClick={move}
          >
            Move
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default MoveAlbumButton
