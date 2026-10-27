import React, { useEffect, useMemo, useRef, useState } from 'react'
import { useParams, useHistory, Link } from 'react-router-dom'
import { useDispatch, useSelector } from 'react-redux'
import {
  Button,
  Card,
  CardContent,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  MenuItem,
  Slider,
  TextField,
  Typography,
} from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import PauseIcon from '@material-ui/icons/Pause'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import SkipNextIcon from '@material-ui/icons/SkipNext'
import SkipPreviousIcon from '@material-ui/icons/SkipPrevious'
import Replay10Icon from '@material-ui/icons/Replay10'
import Forward30Icon from '@material-ui/icons/Forward30'
import { Title, useDataProvider, usePermissions, useRefresh } from 'react-admin'
import { clearQueue } from '../actions'
import subsonic from '../subsonic'
import AudiobookCategoryButton from './AudiobookCategoryButton'

const useStyles = makeStyles((theme) => ({
  root: {
    maxWidth: 1000,
    width: '100%',
    margin: '0 auto',
    padding: theme.spacing(3, 2, 14),
  },
  backLink: {
    color: theme.palette.primary.main,
    '&:visited': { color: theme.palette.primary.main },
  },
  hero: {
    display: 'flex',
    alignItems: 'center',
    flexWrap: 'wrap',
    gap: theme.spacing(4),
    marginBottom: theme.spacing(3),
  },
  art: {
    width: 'min(250px, 55vw)',
    aspectRatio: '1',
    objectFit: 'cover',
    borderRadius: 6,
  },
  controls: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    flexWrap: 'wrap',
  },
  chapter: {
    width: '100%',
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
    justifyContent: 'space-between',
    textAlign: 'left',
    padding: theme.spacing(1.5),
  },
  active: {
    color: theme.palette.primary.main,
    background: 'rgba(230,184,92,.1)',
  },
  timeline: { display: 'flex', alignItems: 'center', gap: theme.spacing(2) },
  slider: { flex: 1 },
}))

const formatTime = (seconds) => {
  const total = Math.floor(seconds || 0)
  return `${Math.floor(total / 3600)}:${String(Math.floor(total / 60) % 60).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`
}

const AudiobookPlayer = () => {
  const { id } = useParams()
  const history = useHistory()
  const classes = useStyles()
  const dataProvider = useDataProvider()
  const { permissions } = usePermissions()
  const refresh = useRefresh()
  const dispatch = useDispatch()
  const musicQueueLength = useSelector(
    (state) => state.player?.queue?.length || 0,
  )
  const audioRef = useRef(null)
  const lastSavedAt = useRef(0)
  const resumeTime = useRef(0)
  const playOnLoad = useRef(false)
  const ended = useRef(false)
  const currentRef = useRef(null)
  const saveQueue = useRef(Promise.resolve())
  const [album, setAlbum] = useState(null)
  const [chapters, setChapters] = useState([])
  const [index, setIndex] = useState(0)
  const [position, setPosition] = useState(0)
  const [duration, setDuration] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [speed, setSpeed] = useState(1)
  const [error, setError] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const chapter = chapters[index]
  const chapterId = chapter?.id
  const streamUrl = useMemo(
    () => (chapterId ? subsonic.streamUrl(chapterId) : ''),
    [chapterId],
  )
  currentRef.current = chapter
    ? {
        trackId: chapter.id,
        positionMs: Math.floor(position * 1000),
        completed,
      }
    : null

  useEffect(() => {
    let active = true
    setAlbum(null)
    setChapters([])
    setError('')
    Promise.all([
      dataProvider.getOne('album', { id }),
      dataProvider.getList('song', {
        pagination: { page: 1, perPage: 0 },
        sort: { field: 'album', order: 'ASC' },
        filter: { album_id: id, missing: false },
      }),
      dataProvider.getAudiobookProgress(id),
    ])
      .then(([book, songs, progress]) => {
        if (!active) return
        if (!book.data.isAudiobook) {
          setError('This album is no longer an audiobook.')
          return
        }
        const ordered = songs.data
          .slice()
          .sort(
            (a, b) =>
              (a.discNumber || 0) - (b.discNumber || 0) ||
              (a.trackNumber || 0) - (b.trackNumber || 0) ||
              a.title.localeCompare(b.title),
          )
        setAlbum(book.data)
        setChapters(ordered)
        const saved = ordered.findIndex(
          (track) => track.id === progress.data.trackId,
        )
        setIndex(saved < 0 ? 0 : saved)
        resumeTime.current =
          saved < 0 ? 0 : (progress.data.positionMs || 0) / 1000
        setPosition(resumeTime.current)
        setCompleted(Boolean(progress.data.completed))
      })
      .catch(() => active && setError('Could not load this audiobook.'))
    return () => {
      active = false
    }
  }, [id, dataProvider])

  useEffect(() => {
    if (musicQueueLength && audioRef.current && !audioRef.current.paused) {
      audioRef.current.pause()
    }
  }, [musicQueueLength])

  useEffect(() => {
    if (audioRef.current) audioRef.current.playbackRate = speed
  }, [speed, index])

  useEffect(
    () => () => {
      const last = currentRef.current
      if (last) {
        saveQueue.current = saveQueue.current
          .catch(() => {})
          .then(() => dataProvider.saveAudiobookProgress(id, last))
          .catch(() => {})
      }
    },
    [id, dataProvider],
  )

  const saveProgress = (track, seconds, done = false) => {
    if (!track) return
    const progress = {
      trackId: track.id,
      positionMs: Math.max(0, Math.floor(seconds * 1000)),
      completed: done,
    }
    saveQueue.current = saveQueue.current
      .catch(() => {})
      .then(() => dataProvider.saveAudiobookProgress(id, progress))
      .catch(() => setError('Could not save audiobook progress.'))
    lastSavedAt.current = Date.now()
  }

  const play = async () => {
    if (!audioRef.current) return
    dispatch(clearQueue())
    setCompleted(false)
    try {
      await audioRef.current.play()
      setPlaying(true)
    } catch {
      setError('Could not play this chapter.')
    }
  }

  const deleteBook = async () => {
    setDeleting(true)
    try {
      await dataProvider.deleteAudiobook(id)
      currentRef.current = null
      ended.current = true
      audioRef.current?.pause()
      try {
        await subsonic.startScan({ target: [`${album.libraryId}:`] })
      } catch {
        // The files and catalogue entry are already gone; scanning can be retried later.
      }
      refresh()
      setDeleting(false)
      history.push('/audiobooks')
    } catch {
      setError('Could not delete this audiobook.')
      setConfirmDelete(false)
      setDeleting(false)
    }
  }

  const selectChapter = (newIndex, autoplay = playing) => {
    if (newIndex < 0 || newIndex >= chapters.length) return
    if (newIndex === index) {
      seek(0)
      if (autoplay) play()
      return
    }
    if (chapter && !ended.current)
      saveProgress(chapter, audioRef.current?.currentTime || position)
    playOnLoad.current = autoplay
    ended.current = true
    resumeTime.current = 0
    setIndex(newIndex)
    setPosition(0)
    setDuration(0)
    setPlaying(false)
  }

  const seek = (seconds) => {
    if (!audioRef.current || !chapter) return
    const target = Math.max(
      0,
      Math.min(seconds, duration || chapter.duration || 0),
    )
    audioRef.current.currentTime = target
    setPosition(target)
    saveProgress(chapter, target)
  }

  const onEnded = () => {
    ended.current = true
    setPlaying(false)
    if (index + 1 < chapters.length) {
      saveProgress(chapters[index + 1], 0)
      currentRef.current = {
        trackId: chapters[index + 1].id,
        positionMs: 0,
        completed: false,
      }
      selectChapter(index + 1, true)
    } else {
      saveProgress(chapter, duration, true)
      currentRef.current = {
        trackId: chapter.id,
        positionMs: Math.floor(duration * 1000),
        completed: true,
      }
      setCompleted(true)
    }
  }

  if (error && !album) return <Typography role="alert">{error}</Typography>
  if (!album) return <Typography>Loading audiobook…</Typography>

  return (
    <div className={classes.root}>
      <Title title={`Music: Server - ${album.name}`} />
      <Link className={classes.backLink} to="/audiobooks">
        ← Audiobooks
      </Link>
      <div className={classes.hero}>
        <img
          className={classes.art}
          src={subsonic.getCoverArtUrl(album, 500)}
          alt=""
        />
        <div>
          <Typography variant="overline" color="primary">
            Audiobook
          </Typography>
          <Typography component="h1" variant="h3">
            {album.name}
          </Typography>
          <Typography color="textSecondary">
            {album.albumArtist} · {chapters.length} chapters
          </Typography>
          <AudiobookCategoryButton album={album} />
          {permissions === 'admin' && (
            <Button color="secondary" onClick={() => setConfirmDelete(true)}>
              Delete book
            </Button>
          )}
        </div>
      </div>
      {chapter && (
        <Card>
          <CardContent>
            <Typography variant="h5">{chapter.title}</Typography>
            <Typography color="textSecondary">
              Chapter {index + 1} of {chapters.length}
              {completed ? ' · Finished' : ''}
            </Typography>
            <audio
              key={chapter.id}
              ref={audioRef}
              preload="metadata"
              src={streamUrl}
              onLoadedMetadata={(event) => {
                const audio = event.currentTarget
                audio.playbackRate = speed
                ended.current = false
                const length = Number.isFinite(audio.duration)
                  ? audio.duration
                  : chapter.duration || 0
                setDuration(length)
                if (resumeTime.current > 0) {
                  audio.currentTime = Math.min(
                    resumeTime.current,
                    Math.max(0, length - 1),
                  )
                  resumeTime.current = 0
                }
                if (playOnLoad.current) {
                  playOnLoad.current = false
                  play()
                }
              }}
              onTimeUpdate={(event) => {
                const seconds = event.currentTarget.currentTime
                setPosition(seconds)
                if (
                  !event.currentTarget.paused &&
                  Date.now() - lastSavedAt.current >= 5000
                ) {
                  saveProgress(chapter, seconds)
                }
              }}
              onPause={(event) => {
                if (audioRef.current !== event.currentTarget) return
                setPlaying(false)
                if (!ended.current)
                  saveProgress(chapter, event.currentTarget.currentTime)
              }}
              onPlay={() => setPlaying(true)}
              onError={() => setError('Could not load this chapter’s audio.')}
              onEnded={onEnded}
            />
            <div className={classes.timeline}>
              <Typography>{formatTime(position)}</Typography>
              <Slider
                className={classes.slider}
                aria-label="Chapter position"
                value={Math.min(position, duration || 0)}
                max={duration || 1}
                onChange={(_, value) => setPosition(value)}
                onChangeCommitted={(_, value) => seek(value)}
              />
              <Typography>
                {formatTime(duration || chapter.duration)}
              </Typography>
            </div>
            <div className={classes.controls}>
              <IconButton
                aria-label="Previous chapter"
                disabled={index === 0}
                onClick={() => selectChapter(index - 1)}
              >
                <SkipPreviousIcon />
              </IconButton>
              <IconButton
                aria-label="Back 10 seconds"
                onClick={() => seek(position - 10)}
              >
                <Replay10Icon />
              </IconButton>
              <Button
                variant="contained"
                color="primary"
                startIcon={playing ? <PauseIcon /> : <PlayArrowIcon />}
                onClick={() => {
                  if (playing) audioRef.current?.pause()
                  else if (completed) {
                    seek(0)
                    play()
                  } else play()
                }}
              >
                {playing
                  ? 'Pause'
                  : completed
                    ? 'Restart'
                    : position > 0
                      ? 'Resume'
                      : 'Play'}
              </Button>
              <IconButton
                aria-label="Forward 30 seconds"
                onClick={() => seek(position + 30)}
              >
                <Forward30Icon />
              </IconButton>
              <IconButton
                aria-label="Next chapter"
                disabled={index + 1 === chapters.length}
                onClick={() => selectChapter(index + 1)}
              >
                <SkipNextIcon />
              </IconButton>
              <TextField
                select
                label="Playback speed"
                value={speed}
                onChange={(event) => setSpeed(Number(event.target.value))}
                id="book-speed"
              >
                {[0.75, 1, 1.25, 1.5, 2].map((rate) => (
                  <MenuItem value={rate} key={rate}>
                    {rate}×
                  </MenuItem>
                ))}
              </TextField>
            </div>
            {error && (
              <Typography role="alert" color="error">
                {error}
              </Typography>
            )}
          </CardContent>
        </Card>
      )}
      <Typography variant="h5" style={{ marginTop: 32 }}>
        Chapters
      </Typography>
      {chapters.map((track, i) => (
        <Button
          key={track.id}
          className={`${classes.chapter} ${i === index ? classes.active : ''}`}
          onClick={() => selectChapter(i)}
        >
          <span>
            {i + 1}. {track.title}
          </span>
          <span>{formatTime(track.duration)}</span>
        </Button>
      ))}
      <Dialog
        open={confirmDelete}
        onClose={() => !deleting && setConfirmDelete(false)}
        aria-labelledby="delete-book-title"
      >
        <DialogTitle id="delete-book-title">Delete audiobook?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Permanently delete {album.name}, its chapters and its cover files
            from the server? This cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button disabled={deleting} onClick={() => setConfirmDelete(false)}>
            Cancel
          </Button>
          <Button color="secondary" disabled={deleting} onClick={deleteBook}>
            Delete book
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default AudiobookPlayer
