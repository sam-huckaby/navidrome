import React from 'react'
import { Link } from 'react-router-dom'
import { Typography } from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import { Title as RaTitle, useListContext } from 'react-admin'
import { List } from '../common'
import subsonic from '../subsonic'

const useStyles = makeStyles((theme) => ({
  root: { padding: theme.spacing(2), maxWidth: 1200, margin: '0 auto' },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))',
    gap: theme.spacing(3),
  },
  card: {
    textDecoration: 'none',
    color: theme.palette.text.primary,
    display: 'block',
    minWidth: 0,
  },
  cover: {
    width: '100%',
    aspectRatio: '1',
    objectFit: 'cover',
    borderRadius: 5,
    background: theme.palette.background.paper,
  },
  title: {
    fontWeight: 600,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  author: {
    color: theme.palette.text.secondary,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
}))

const AudiobookGrid = () => {
  const classes = useStyles()
  const { ids = [], data = {}, loading } = useListContext()
  return (
    <div className={classes.root}>
      <Typography variant="h4" component="h1" gutterBottom>
        Audiobooks
      </Typography>
      {!loading && ids.length === 0 && (
        <Typography>
          No audiobooks yet. Mark an album as an audiobook or upload one.
        </Typography>
      )}
      <div className={classes.grid}>
        {ids.map((id) => {
          const album = data[id]
          if (!album) return null
          return (
            <Link
              className={classes.card}
              to={`/audiobooks/${album.id}`}
              key={id}
            >
              <img
                className={classes.cover}
                src={subsonic.getCoverArtUrl(album, 350)}
                alt=""
                loading="lazy"
              />
              <Typography className={classes.title}>{album.name}</Typography>
              <Typography className={classes.author}>
                {album.albumArtist}
              </Typography>
            </Link>
          )
        })}
      </div>
    </div>
  )
}

const AudiobookLibrary = () => (
  <>
    <RaTitle title="Music: Server - Audiobooks" />
    <List
      resource="album"
      basePath="/album"
      filter={{ audiobook: true }}
      sort={{ field: 'name', order: 'ASC' }}
      perPage={36}
      exporter={false}
      bulkActionButtons={false}
      actions={null}
      title="Audiobooks"
    >
      <AudiobookGrid />
    </List>
  </>
)

export default AudiobookLibrary
