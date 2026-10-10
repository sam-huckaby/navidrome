import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import UploadMusic from './upload/UploadMusic'
import AudiobookLibrary from './audiobook/AudiobookLibrary'
import AudiobookPlayer from './audiobook/AudiobookPlayer'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/upload" render={() => <UploadMusic />} key={'upload'} />,
  <Route
    exact
    path="/audiobooks"
    render={() => <AudiobookLibrary />}
    key={'audiobooks'}
  />,
  <Route
    exact
    path="/audiobooks/:id"
    render={() => <AudiobookPlayer />}
    key={'audiobook-player'}
  />,
]

export default routes
