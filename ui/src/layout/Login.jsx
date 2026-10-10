import React, { useState, useCallback } from 'react'
import PropTypes from 'prop-types'
import { Field, Form } from 'react-final-form'
import { useDispatch } from 'react-redux'
import Button from '@material-ui/core/Button'
import Card from '@material-ui/core/Card'
import CardActions from '@material-ui/core/CardActions'
import CircularProgress from '@material-ui/core/CircularProgress'
import Link from '@material-ui/core/Link'
import TextField from '@material-ui/core/TextField'
import { ThemeProvider, makeStyles } from '@material-ui/core/styles'
import {
  createMuiTheme,
  useLogin,
  useNotify,
  useTranslate,
  useVersion,
} from 'react-admin'
import { MusicMark } from '../icons/MusicBrand'

import Notification from './Notification'
import useCurrentTheme from '../themes/useCurrentTheme'
import config from '../config'
import { clearQueue } from '../actions'
import { INSIGHTS_DOC_URL } from '../consts.js'

const useStyles = makeStyles(
  () => ({
    main: {
      display: 'grid',
      gridTemplateColumns: 'minmax(300px, 1fr) minmax(320px, 1fr)',
      minHeight: '100vh',
      background: '#11110f',
      '@media (max-width: 760px)': { gridTemplateColumns: '1fr' },
    },
    story: {
      position: 'relative',
      display: 'flex',
      flexDirection: 'column',
      justifyContent: 'center',
      padding: 'clamp(40px, 8vw, 120px)',
      overflow: 'hidden',
      background:
        'radial-gradient(circle at 10% 90%, #65482a, transparent 40%), linear-gradient(135deg, #1d1b16, #11110f)',
      '&:after': {
        content: '""',
        position: 'absolute',
        width: 470,
        height: 470,
        right: -180,
        bottom: -220,
        borderRadius: '50%',
        border: '1px solid rgba(230,184,92,.18)',
        boxShadow:
          '0 0 0 65px rgba(230,184,92,.04), 0 0 0 140px rgba(230,184,92,.025)',
        pointerEvents: 'none',
      },
      '@media (max-width: 760px)': { minHeight: 220, padding: '36px 28px' },
    },
    storyTitle: {
      fontFamily: '"Newsreader", Georgia, serif',
      fontSize: 'clamp(54px, 7vw, 100px)',
      lineHeight: 0.95,
      letterSpacing: '-.04em',
      margin: '25px 0 10px',
      '@media (max-width: 760px)': { fontSize: 52, margin: '12px 0 0' },
    },
    panel: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      padding: '40px 24px',
      background: '#171714',
    },
    card: {
      width: 'min(440px, 100%)',
      padding: '28px 22px',
      overflow: 'visible',
      background: '#191916',
      border: '1px solid rgba(255,255,255,.09)',
    },
    avatar: {
      display: 'flex',
      justifyContent: 'center',
      marginBottom: 18,
    },
    systemName: {
      display: 'flex',
      justifyContent: 'center',
      color: '#f3eee4',
      fontFamily: '"Newsreader", Georgia, serif',
      fontSize: 30,
    },
    welcome: {
      marginTop: '1em',
      padding: '0 1em',
      display: 'flex',
      justifyContent: 'center',
      flexWrap: 'wrap',
      color: '#aaa79f',
    },
    form: {
      padding: '0 1em 1em 1em',
    },
    input: {
      marginTop: '1em',
    },
    actions: {
      padding: '0 1em 1em 1em',
    },
    button: { marginTop: 10 },
    systemNameLink: {
      textDecoration: 'none',
    },
    message: {
      marginTop: '1em',
      padding: '0 1em 1em 1em',
      textAlign: 'center',
      wordBreak: 'break-word',
      fontSize: '0.875em',
    },
  }),
  { name: 'NDLogin' },
)

const renderInput = ({
  meta: { touched, error } = {},
  input: { ...inputProps },
  ...props
}) => (
  <TextField
    error={!!(touched && error)}
    inputProps={{
      // mobile keyboards: suppress capitalization and correction for login related fields
      autocapitalize: 'none',
      autocorrect: 'off',
      ...inputProps,
    }}
    helperText={touched && error}
    {...props}
    fullWidth
  />
)

const LoginStory = ({ classes }) => (
  <aside className={classes.story}>
    <MusicMark large />
    <h1 className={classes.storyTitle}>
      Music:
      <br />
      Server.
    </h1>
    <p>Your music, from anywhere.</p>
  </aside>
)

const FormLogin = ({ loading, handleSubmit, validate }) => {
  const translate = useTranslate()
  const classes = useStyles()

  return (
    <Form
      onSubmit={handleSubmit}
      validate={validate}
      render={({ handleSubmit }) => (
        <form onSubmit={handleSubmit} noValidate>
          <div className={classes.main}>
            <LoginStory classes={classes} />
            <div className={classes.panel}>
              <Card className={classes.card}>
                <div className={classes.avatar}>
                  <MusicMark large />
                </div>
                <div className={classes.systemName}>
                  <a
                    href="https://www.navidrome.org"
                    target="_blank"
                    rel="noopener noreferrer"
                    className={classes.systemNameLink}
                  >
                    Music: Server
                  </a>
                </div>
                {config.welcomeMessage && (
                  <div
                    className={classes.welcome}
                    // Use dangerouslySetInnerHTML to allow admins to configure
                    // whatever content they want
                    dangerouslySetInnerHTML={{ __html: config.welcomeMessage }}
                  />
                )}
                <div className={classes.form}>
                  <div className={classes.input}>
                    <Field
                      autoFocus
                      name="username"
                      component={renderInput}
                      label={translate('ra.auth.username')}
                      disabled={loading}
                      spellCheck={false}
                    />
                  </div>
                  <div className={classes.input}>
                    <Field
                      name="password"
                      component={renderInput}
                      label={translate('ra.auth.password')}
                      type="password"
                      disabled={loading}
                    />
                  </div>
                </div>
                <CardActions className={classes.actions}>
                  <Button
                    variant="contained"
                    type="submit"
                    color="primary"
                    disabled={loading}
                    className={classes.button}
                    fullWidth
                  >
                    {loading && <CircularProgress size={25} thickness={2} />}
                    {translate('ra.auth.sign_in')}
                  </Button>
                </CardActions>
              </Card>
              <Notification />
            </div>
          </div>
        </form>
      )}
    />
  )
}

const InsightsNotice = ({ url }) => {
  const translate = useTranslate()
  const classes = useStyles()

  const anchorRegex = /\[(.+?)]/g
  const originalMsg = translate('ra.auth.insightsCollectionNote')

  // Split the entire message on newlines
  const lines = originalMsg.split('\n')

  const renderedLines = lines.map((line, lineIndex) => {
    const segments = []
    let lastIndex = 0
    let match

    // Find bracketed text in each line
    while ((match = anchorRegex.exec(line)) !== null) {
      // match.index is where "[something]" starts
      // match[1] is the text inside the brackets
      const bracketText = match[1]

      // Push the text before the bracket
      segments.push(line.slice(lastIndex, match.index))

      // Push the <Link> component
      segments.push(
        <Link
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          key={`${lineIndex}-${match.index}`}
          style={{ cursor: 'pointer' }}
        >
          {bracketText}
        </Link>,
      )

      // Update lastIndex to the character right after the bracketed text
      lastIndex = match.index + match[0].length
    }

    // Push the remaining text after the last bracket
    segments.push(line.slice(lastIndex))

    // Return this line’s parts, plus a <br/> if not the last line
    return (
      <React.Fragment key={lineIndex}>
        {segments}
        {lineIndex < lines.length - 1 && <br />}
      </React.Fragment>
    )
  })

  return <div className={classes.message}>{renderedLines}</div>
}

const FormSignUp = ({ loading, handleSubmit, validate }) => {
  const translate = useTranslate()
  const classes = useStyles()

  return (
    <Form
      onSubmit={handleSubmit}
      validate={validate}
      render={({ handleSubmit }) => (
        <form onSubmit={handleSubmit} noValidate>
          <div className={classes.main}>
            <LoginStory classes={classes} />
            <div className={classes.panel}>
              <Card className={classes.card}>
                <div className={classes.avatar}>
                  <MusicMark large />
                </div>
                <div className={classes.welcome}>
                  {translate('ra.auth.welcome1')}
                </div>
                <div className={classes.welcome}>
                  {translate('ra.auth.welcome2')}
                </div>
                <div className={classes.form}>
                  <div className={classes.input}>
                    <Field
                      autoFocus
                      name="username"
                      component={renderInput}
                      label={translate('ra.auth.username')}
                      disabled={loading}
                      spellCheck={false}
                    />
                  </div>
                  <div className={classes.input}>
                    <Field
                      name="password"
                      component={renderInput}
                      label={translate('ra.auth.password')}
                      type="password"
                      disabled={loading}
                    />
                  </div>
                  <div className={classes.input}>
                    <Field
                      name="confirmPassword"
                      component={renderInput}
                      label={translate('ra.auth.confirmPassword')}
                      type="password"
                      disabled={loading}
                    />
                  </div>
                </div>
                <CardActions className={classes.actions}>
                  <Button
                    variant="contained"
                    type="submit"
                    color="primary"
                    disabled={loading}
                    className={classes.button}
                    fullWidth
                  >
                    {loading && <CircularProgress size={25} thickness={2} />}
                    {translate('ra.auth.buttonCreateAdmin')}
                  </Button>
                </CardActions>
                <InsightsNotice url={INSIGHTS_DOC_URL} />
              </Card>
              <Notification />
            </div>
          </div>
        </form>
      )}
    />
  )
}

const Login = ({ location }) => {
  const [loading, setLoading] = useState(false)
  const translate = useTranslate()
  const notify = useNotify()
  const login = useLogin()
  const dispatch = useDispatch()

  const handleSubmit = useCallback(
    (auth) => {
      setLoading(true)
      dispatch(clearQueue())
      login(auth, location.state ? location.state.nextPathname : '/').catch(
        (error) => {
          setLoading(false)
          notify(
            typeof error === 'string'
              ? error
              : typeof error === 'undefined' || !error.message
                ? 'ra.auth.sign_in_error'
                : error.message,
            'warning',
          )
        },
      )
    },
    [dispatch, login, notify, setLoading, location],
  )

  const validateLogin = useCallback(
    (values) => {
      const errors = {}
      if (!values.username) {
        errors.username = translate('ra.validation.required')
      }
      if (!values.password) {
        errors.password = translate('ra.validation.required')
      }
      return errors
    },
    [translate],
  )

  const validateSignup = useCallback(
    (values) => {
      const errors = validateLogin(values)
      const regex = /^\w+$/g
      if (values.username && !values.username.match(regex)) {
        errors.username = translate('ra.validation.invalidChars')
      }
      if (!values.confirmPassword) {
        errors.confirmPassword = translate('ra.validation.required')
      }
      if (values.confirmPassword !== values.password) {
        errors.confirmPassword = translate('ra.validation.passwordDoesNotMatch')
      }
      return errors
    },
    [translate, validateLogin],
  )

  if (config.firstTime) {
    return (
      <FormSignUp
        handleSubmit={handleSubmit}
        validate={validateSignup}
        loading={loading}
      />
    )
  }
  return (
    <FormLogin
      handleSubmit={handleSubmit}
      validate={validateLogin}
      loading={loading}
    />
  )
}

Login.propTypes = {
  authProvider: PropTypes.func,
  previousRoute: PropTypes.string,
}

// We need to put the ThemeProvider decoration in another component
// Because otherwise the useStyles() hook used in Login won't get
// the right theme
const LoginWithTheme = (props) => {
  const theme = useCurrentTheme()
  const version = useVersion()

  return (
    <ThemeProvider theme={createMuiTheme(theme)}>
      <Login key={version} {...props} />
    </ThemeProvider>
  )
}

export default LoginWithTheme
