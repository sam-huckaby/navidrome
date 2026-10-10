import React, { useState } from 'react'
import { Button } from '@material-ui/core'
import { useDataProvider, useNotify, usePermissions } from 'react-admin'
import { useHistory } from 'react-router-dom'

const AudiobookCategoryButton = ({ album }) => {
  const { permissions } = usePermissions()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const history = useHistory()
  const [busy, setBusy] = useState(false)
  if (permissions !== 'admin' || !album?.id) return null

  const audiobook = Boolean(album.isAudiobook)
  return (
    <Button
      size="small"
      disabled={busy}
      onClick={async () => {
        setBusy(true)
        try {
          await dataProvider.setAudiobookCategory(album.id, !audiobook)
          history.push(
            audiobook ? `/album/${album.id}/show` : `/audiobooks/${album.id}`,
          )
        } catch {
          notify('Could not change the album category', 'warning')
        } finally {
          setBusy(false)
        }
      }}
    >
      {audiobook ? 'Mark as music' : 'Mark as audiobook'}
    </Button>
  )
}

export default AudiobookCategoryButton
