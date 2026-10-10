import React, { useEffect, useState } from 'react'
import {
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@material-ui/core'
import Autocomplete from '@material-ui/lab/Autocomplete'
import EditIcon from '@material-ui/icons/Edit'
import {
  useDataProvider,
  useNotify,
  usePermissions,
  useRefresh,
} from 'react-admin'

const fields = [
  ['genre', 'Genres'],
  ['mood', 'Mood'],
  ['grouping', 'Grouping'],
  ['releasetype', 'Release type'],
  ['recordlabel', 'Record label'],
  ['media', 'Media'],
  ['albumversion', 'Album version'],
]

const valuesFor = (tags) =>
  Object.fromEntries(fields.map(([key]) => [key, tags?.[key] || []]))

const EditAlbumTags = ({ record }) => {
  const { permissions } = usePermissions()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const refresh = useRefresh()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [original, setOriginal] = useState(null)
  const [draft, setDraft] = useState(null)
  const [options, setOptions] = useState({})
  const [loadError, setLoadError] = useState(false)
  const [loadAttempt, setLoadAttempt] = useState(0)

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    setLoadError(false)
    Promise.all([
      dataProvider.getAlbumTags(record.id),
      dataProvider
        .getList('tag', {
          pagination: { page: 1, perPage: 1000 },
          sort: { field: 'name', order: 'ASC' },
          filter: {},
        })
        .catch(() => ({ data: [] })),
    ])
      .then(([{ data: album }, { data: tags }]) => {
        if (!active) return
        const values = valuesFor(album.tags)
        setOriginal(values)
        setDraft(values)
        const suggestions = {}
        for (const tag of tags) {
          if (!suggestions[tag.tagName]) suggestions[tag.tagName] = []
          suggestions[tag.tagName].push(tag.tagValue)
        }
        setOptions(suggestions)
      })
      .catch(() => {
        if (active) setLoadError(true)
      })
      .finally(() => active && setLoading(false))
    return () => {
      active = false
    }
  }, [open, record.id, dataProvider, loadAttempt])

  if (permissions !== 'admin' || !record?.id || record.missing) return null

  const save = async () => {
    const changes = {}
    for (const [key] of fields) {
      if (JSON.stringify(draft[key]) !== JSON.stringify(original[key])) {
        changes[key] = draft[key]
      }
    }
    if (!Object.keys(changes).length) {
      setOpen(false)
      return
    }
    setSaving(true)
    try {
      await dataProvider.updateAlbumTags(record.id, changes)
      setOpen(false)
      refresh()
      notify('Album tags saved', 'info')
    } catch {
      notify('Could not save album tags', 'warning')
    } finally {
      setSaving(false)
    }
  }

  const restore = async () => {
    setSaving(true)
    try {
      await dataProvider.clearAlbumTags(record.id)
      setOpen(false)
      refresh()
      notify('Album tags restored from tracks', 'info')
    } catch {
      notify('Could not restore album tags', 'warning')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Button
        onClick={() => {
          setDraft(null)
          setOpen(true)
        }}
        startIcon={<EditIcon />}
        aria-label="Edit album tags"
      >
        Edit tags
      </Button>
      <Dialog
        open={open}
        onClose={() => !saving && setOpen(false)}
        fullWidth
        maxWidth="sm"
        aria-labelledby="edit-album-tags-title"
      >
        <DialogTitle id="edit-album-tags-title">Edit album tags</DialogTitle>
        <DialogContent dividers>
          <Typography color="textSecondary" gutterBottom>
            Changes apply to this album on Music: Server. Track files and song
            genres stay unchanged. Press Enter to add a new value.
          </Typography>
          {loading ? (
            <CircularProgress aria-label="Loading album tags" />
          ) : loadError ? (
            <div role="alert">
              <Typography>Could not load album tags.</Typography>
              <Button onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
                Retry
              </Button>
            </div>
          ) : !draft ? null : (
            fields.map(([key, label]) => (
              <Autocomplete
                key={key}
                multiple
                freeSolo
                filterSelectedOptions
                options={options[key] || []}
                value={draft[key]}
                onChange={(_, values) =>
                  setDraft((prev) => ({ ...prev, [key]: values }))
                }
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label={label}
                    variant="outlined"
                    margin="normal"
                    fullWidth
                  />
                )}
              />
            ))
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={restore} disabled={saving || loading || !draft}>
            Restore from tracks
          </Button>
          <Button onClick={() => setOpen(false)} disabled={saving}>
            Cancel
          </Button>
          <Button
            color="primary"
            variant="contained"
            onClick={save}
            disabled={saving || loading || !draft}
          >
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default EditAlbumTags
