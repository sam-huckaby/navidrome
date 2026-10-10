import React, { useEffect, useState } from 'react'
import { IconButton, Tooltip } from '@material-ui/core'
import WarningOutlinedIcon from '@material-ui/icons/WarningOutlined'
import { useDataProvider, usePermissions } from 'react-admin'

const FingerprintWarning = () => {
  const { permissions } = usePermissions()
  const dataProvider = useDataProvider()
  const [status, setStatus] = useState(null)

  useEffect(() => {
    if (permissions !== 'admin') return
    let mounted = true
    dataProvider
      .musicUploadStatus()
      .then(({ data }) => mounted && setStatus(data))
      .catch(() => {})
    return () => {
      mounted = false
    }
  }, [dataProvider, permissions])

  if (
    permissions !== 'admin' ||
    !status ||
    (status.fpcalcInstalled && status.matchingConfigured)
  )
    return null

  const message = !status.fpcalcInstalled
    ? 'The fingerprinting library is not installed, so missing metadata will not be updated automatically.'
    : 'AcoustID is not configured, so missing metadata will not be updated automatically.'
  return (
    <Tooltip title={message}>
      <IconButton
        aria-label={message}
        style={{ color: '#e6b85c' }}
        size="small"
      >
        <WarningOutlinedIcon />
      </IconButton>
    </Tooltip>
  )
}

export default FingerprintWarning
