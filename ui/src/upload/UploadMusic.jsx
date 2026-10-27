import React, { useEffect, useState } from 'react'
import {
  Button,
  Card,
  CardContent,
  Checkbox,
  FormControlLabel,
  LinearProgress,
  MenuItem,
  TextField,
  Typography,
  makeStyles,
} from '@material-ui/core'
import CloudUploadIcon from '@material-ui/icons/CloudUpload'
import { Title, useDataProvider, usePermissions } from 'react-admin'
import subsonic from '../subsonic'

const useStyles = makeStyles((theme) => ({
  root: { maxWidth: 960, width: '100%', margin: '24px auto 100px' },
  card: { padding: theme.spacing(2), marginBottom: theme.spacing(2) },
  actions: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: theme.spacing(2),
    marginTop: theme.spacing(2),
  },
  track: {
    padding: theme.spacing(2),
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  fields: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(170px, 1fr))',
    gap: theme.spacing(1),
  },
  subtitle: {
    color: theme.palette.text.secondary,
    margin: theme.spacing(1, 0),
  },
}))

const uploadExtensions = new Set([
  'mp3',
  'flac',
  'm4a',
  'aac',
  'ogg',
  'opus',
  'wav',
  'aiff',
  'aif',
  'wma',
  'ape',
  'wv',
  'dsf',
  'jpg',
  'jpeg',
  'png',
  'webp',
  'lrc',
  'elrc',
  'ttml',
  'srt',
  'txt',
  'cue',
  'yaml',
  'yml',
])

const albumSourceGroup = ({ path, album }) => {
  const parts = path.split('/')
  parts.pop()
  if (/^(cd|disc|disk)[ _.-]?\d+$/i.test(parts[parts.length - 1])) parts.pop()
  return `${parts.join('/')}\0${album}`
}

const uploadAlbumKey = (track) =>
  `${albumSourceGroup(track)}\0${track.albumArtist || track.artist}`

const groupCompilations = (tracks) => {
  const artists = new Map()
  for (const track of tracks) {
    if (!track.album || !track.artist || track.albumArtist) continue
    const key = albumSourceGroup(track)
    if (!artists.has(key)) artists.set(key, new Set())
    artists.get(key).add(track.artist)
  }
  for (const track of tracks) {
    if (!track.albumArtist && artists.get(albumSourceGroup(track))?.size > 1) {
      track.albumArtist = 'Various Artists'
    }
  }
}

const UploadMusic = () => {
  const classes = useStyles()
  const { permissions } = usePermissions()
  const dataProvider = useDataProvider()
  const [libraries, setLibraries] = useState([])
  const [libraryId, setLibraryId] = useState('')
  const [status, setStatus] = useState(null)
  const [files, setFiles] = useState([])
  const [unsupportedFiles, setUnsupportedFiles] = useState(0)
  const [sessionId, setSessionId] = useState(null)
  const [tracks, setTracks] = useState([])
  const [originalTags, setOriginalTags] = useState({})
  const [candidates, setCandidates] = useState({})
  const [result, setResult] = useState(null)
  const [progress, setProgress] = useState(0)
  const [phase, setPhase] = useState('select')
  const [error, setError] = useState('')

  useEffect(() => {
    if (permissions !== 'admin') return
    dataProvider
      .getList('library', {
        pagination: { page: 1, perPage: 1000 },
        sort: { field: 'name', order: 'ASC' },
        filter: {},
      })
      .then(({ data }) => {
        setLibraries(data)
        if (data.length)
          setLibraryId(
            String(data.find((lib) => lib.id === 1)?.id || data[0].id),
          )
      })
      .catch(() => setError('Could not load music libraries.'))
    dataProvider
      .musicUploadStatus()
      .then(({ data }) => setStatus(data))
      .catch(() => {})
  }, [permissions, dataProvider])

  if (permissions !== 'admin')
    return <Typography>Only administrators can upload music.</Typography>

  const updateTrack = (index, patch) =>
    setTracks((list) =>
      list.map((track, i) => (i === index ? { ...track, ...patch } : track)),
    )

  const begin = async () => {
    setError('')
    setPhase('uploading')
    try {
      const { data: session } = await dataProvider.createMusicUpload(
        Number(libraryId),
      )
      setSessionId(session.id)
      for (let i = 0; i < files.length; i++) {
        await dataProvider.sendMusicUploadFile(session.id, files[i])
        setProgress(Math.round(((i + 1) / files.length) * 100))
      }
      setPhase('analyzing')
      const { data: preview } = await dataProvider.previewMusicUpload(
        session.id,
      )
      const nextTracks = preview.tracks
      setOriginalTags(
        Object.fromEntries(
          nextTracks.map((track) => [track.path, { ...track }]),
        ),
      )
      if (status?.fpcalcInstalled && status?.matchingConfigured) {
        for (let i = 0; i < nextTracks.length; i++) {
          if (!nextTracks[i].needsMatch) continue
          try {
            const { data } = await dataProvider.matchMusicUploadTrack(
              session.id,
              nextTracks[i].path,
            )
            const choices = data.candidates || []
            setCandidates((prev) => ({
              ...prev,
              [nextTracks[i].path]: choices,
            }))
            const best = choices[0]
            const runnerUp = choices[1]
            if (
              best &&
              best.score >= 0.98 &&
              (!runnerUp || runnerUp.score < best.score - 0.1) &&
              (!nextTracks[i].album ||
                !best.album ||
                nextTracks[i].album.toLowerCase() === best.album.toLowerCase())
            ) {
              nextTracks[i] = {
                ...nextTracks[i],
                title: nextTracks[i].title || best.title,
                artist: nextTracks[i].artist || best.artist,
                album: nextTracks[i].album || best.album,
              }
            }
          } catch {
            // Missing tools, no match or a temporary provider failure never blocks upload.
          }
        }
      }
      groupCompilations(nextTracks)
      setTracks(nextTracks)
      setPhase('review')
    } catch (err) {
      setError(
        err.message || 'Upload failed. Your local files were not changed.',
      )
      setPhase('failed')
    }
  }

  const finish = async () => {
    setError('')
    setPhase('importing')
    try {
      const { data } = await dataProvider.importMusicUpload(sessionId, tracks)
      setResult(data)
      if (data.imported.length) {
        try {
          await subsonic.startScan({ target: [`${data.libraryId}:`] })
        } catch {
          setError(
            'Files were imported, but scanning could not be started. Run a library scan to see them.',
          )
        }
      }
      setPhase('complete')
    } catch (err) {
      setError(err.message || 'Could not import the music.')
      setPhase('review')
    }
  }

  const reset = async () => {
    if (sessionId) {
      try {
        await dataProvider.cancelMusicUpload(sessionId)
      } catch {
        /* Session cleanup is best effort. */
      }
    }
    setFiles([])
    setUnsupportedFiles(0)
    setTracks([])
    setOriginalTags({})
    setCandidates({})
    setProgress(0)
    setSessionId(null)
    setPhase('select')
    setResult(null)
    setError('')
  }

  const incomplete = tracks.some(
    (track) =>
      !track.title.trim() || !track.artist.trim() || !track.album.trim(),
  )

  const uploadAlbums = Array.from(
    new Map(tracks.map((track) => [uploadAlbumKey(track), track])).entries(),
  )

  return (
    <div className={classes.root}>
      <Title title="Music: Server - Upload Music" />
      <Typography variant="h4" component="h1">
        Upload Music
      </Typography>
      <Typography className={classes.subtitle}>
        Select a folder on your computer. Music is organized by Album Artist /
        Album in the library you choose.
      </Typography>
      {error && (
        <Typography role="alert" color="error">
          {error}
        </Typography>
      )}
      {phase === 'select' && (
        <Card className={classes.card}>
          <CardContent>
            <TextField
              id="upload-library"
              select
              fullWidth
              label="Destination library"
              value={libraryId}
              onChange={(e) => setLibraryId(e.target.value)}
            >
              {libraries.map((lib) => (
                <MenuItem value={String(lib.id)} key={lib.id}>
                  {lib.name}
                </MenuItem>
              ))}
            </TextField>
            <div className={classes.actions}>
              <input
                aria-label="Choose a music folder"
                type="file"
                webkitdirectory=""
                multiple
                onChange={(e) => {
                  const selected = Array.from(e.target.files)
                  const accepted = selected.filter((file) =>
                    uploadExtensions.has(
                      file.name.split('.').pop().toLowerCase(),
                    ),
                  )
                  setFiles(accepted)
                  setUnsupportedFiles(selected.length - accepted.length)
                }}
              />
            </div>
            {files.length > 0 && (
              <Typography>
                {files.length} files selected (
                {(files.reduce((n, f) => n + f.size, 0) / 1048576).toFixed(1)}{' '}
                MB)
              </Typography>
            )}
            {unsupportedFiles > 0 && (
              <Typography>
                {unsupportedFiles} unsupported files will be skipped.
              </Typography>
            )}
            <div className={classes.actions}>
              <Button
                color="primary"
                variant="contained"
                startIcon={<CloudUploadIcon />}
                disabled={!files.length || !libraryId}
                onClick={begin}
              >
                Upload folder
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {(phase === 'uploading' ||
        phase === 'analyzing' ||
        phase === 'importing') && (
        <Card className={classes.card}>
          <CardContent>
            <Typography>
              {phase === 'uploading'
                ? `Uploading files: ${progress}%`
                : phase === 'analyzing'
                  ? 'Reading tags and looking up missing metadata…'
                  : 'Importing music into your library…'}
            </Typography>
            <LinearProgress
              variant={phase === 'uploading' ? 'determinate' : 'indeterminate'}
              value={progress}
            />
          </CardContent>
        </Card>
      )}
      {phase === 'failed' && <Button onClick={reset}>Start over</Button>}
      {phase === 'review' && (
        <Card className={classes.card}>
          <CardContent>
            <Typography variant="h5">Review metadata</Typography>
            <Typography className={classes.subtitle}>
              Missing details must be filled in before import. Fields already
              stored in the audio files are read-only and kept unchanged.
            </Typography>
            {uploadAlbums.map(([key, album]) => (
              <div key={key}>
                <FormControlLabel
                  control={
                    <Checkbox
                      color="primary"
                      checked={tracks.some(
                        (track) =>
                          uploadAlbumKey(track) === key && track.isAudiobook,
                      )}
                      onChange={(event) => {
                        const isAudiobook = event.target.checked
                        setTracks((list) =>
                          list.map((track) =>
                            uploadAlbumKey(track) === key
                              ? { ...track, isAudiobook }
                              : track,
                          ),
                        )
                      }}
                    />
                  }
                  label={`${album.album || 'Untitled album'} — Audiobook`}
                />
              </div>
            ))}
            {!status?.fpcalcInstalled && (
              <Typography color="textSecondary">
                Fingerprinting is unavailable; enter missing details manually.
              </Typography>
            )}
            {tracks.map((track, index) => (
              <div className={classes.track} key={track.path}>
                <Typography variant="subtitle2">{track.path}</Typography>
                {(candidates[track.path] || []).length > 0 && (
                  <TextField
                    id={`upload-match-${index}`}
                    select
                    label="Suggested matches"
                    fullWidth
                    value=""
                    onChange={(e) => {
                      const choice =
                        candidates[track.path][Number(e.target.value)]
                      updateTrack(index, {
                        title: track.title || choice.title,
                        artist: track.artist || choice.artist,
                        album: track.album || choice.album,
                      })
                    }}
                  >
                    <MenuItem value="">
                      Select a match to fill missing fields
                    </MenuItem>
                    {candidates[track.path].map((candidate, i) => (
                      <MenuItem key={i} value={i}>
                        {candidate.title} — {candidate.artist} ·{' '}
                        {candidate.album} ({Math.round(candidate.score * 100)}%)
                      </MenuItem>
                    ))}
                  </TextField>
                )}
                <div className={classes.fields}>
                  {['title', 'artist', 'album', 'albumArtist'].map((field) => (
                    <TextField
                      key={field}
                      id={`upload-${index}-${field}`}
                      label={
                        {
                          title: 'Title',
                          artist: 'Artist',
                          album: 'Album',
                          albumArtist: 'Album Artist',
                        }[field]
                      }
                      value={track[field] || ''}
                      InputProps={{
                        readOnly:
                          Boolean(originalTags[track.path]?.[field]) &&
                          !(
                            field === 'albumArtist' &&
                            originalTags[track.path]?.[field] ===
                              'Various Artists'
                          ),
                      }}
                      onChange={(e) =>
                        updateTrack(index, { [field]: e.target.value })
                      }
                    />
                  ))}
                </div>
              </div>
            ))}
            <div className={classes.actions}>
              <Button onClick={reset}>Cancel upload</Button>
              <Button
                color="primary"
                variant="contained"
                disabled={incomplete || !tracks.length}
                onClick={finish}
              >
                Import music
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {phase === 'complete' && (
        <Card className={classes.card}>
          <CardContent>
            <Typography variant="h5">Import complete</Typography>
            <Typography>
              {result.imported.length} files imported; {result.skipped.length}{' '}
              already existed; {result.unmatched.length} companion files could
              not be assigned.
            </Typography>
            {result.skipped.length > 0 && (
              <Typography className={classes.subtitle}>
                Skipped: {result.skipped.join(', ')}
              </Typography>
            )}
            {result.unmatched.length > 0 && (
              <Typography className={classes.subtitle}>
                Unassigned: {result.unmatched.join(', ')}
              </Typography>
            )}
            <Button onClick={reset}>Upload another folder</Button>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

export default UploadMusic
