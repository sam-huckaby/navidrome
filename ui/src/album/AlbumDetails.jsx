import { useEffect, useState } from 'react'
import {
  Card,
  CardContent,
  Collapse,
  makeStyles,
  Typography,
  useMediaQuery,
  withWidth,
} from '@material-ui/core'
import {
  ArrayField,
  ChipField,
  Link,
  SingleFieldList,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import clsx from 'clsx'
import Lightbox from 'react-image-lightbox'
import config from '../config'
import 'react-image-lightbox/style.css'
import subsonic from '../subsonic'
import { Artwork } from '../common/Artwork'
import {
  ArtistLinkField,
  CollapsibleComment,
  DurationField,
  formatRange,
  LoveButton,
  RatingField,
  SizeField,
  useAlbumsPerPage,
} from '../common'
import { useDateLocale } from '../i18n/useDateLocale'
import { formatFullDate, intersperse } from '../utils'
import AlbumExternalLinks from './AlbumExternalLinks'
import EditAlbumTags from './EditAlbumTags'
import AudiobookCategoryButton from '../audiobook/AudiobookCategoryButton'
import { SafeHTML } from '../common/SafeHTML'

const useStyles = makeStyles(
  (theme) => ({
    root: {
      marginBottom: theme.spacing(3),
      background:
        'radial-gradient(ellipse at 5% 100%, rgba(183,128,55,.14), transparent 50%), #191916',
      [theme.breakpoints.down('xs')]: {
        padding: theme.spacing(2),
        minWidth: 0,
      },
      [theme.breakpoints.up('sm')]: {
        padding: theme.spacing(3),
        minWidth: 0,
      },
    },
    cardContents: {
      display: 'flex',
      alignItems: 'flex-end',
      gap: theme.spacing(3),
      [theme.breakpoints.down('xs')]: { gap: theme.spacing(2) },
    },
    details: {
      display: 'flex',
      flexDirection: 'column',
      minWidth: 0,
    },
    content: {
      flex: '1 1 auto',
      padding: '0 !important',
    },
    coverParent: {
      [theme.breakpoints.down('xs')]: {
        height: '8em',
        width: '8em',
        minWidth: '8em',
      },
      [theme.breakpoints.up('sm')]: {
        height: '10em',
        width: '10em',
        minWidth: '10em',
      },
      [theme.breakpoints.up('lg')]: {
        height: '15em',
        width: '15em',
        minWidth: '15em',
      },
      backgroundColor: 'transparent',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      flexShrink: 0,
      boxShadow: '0 20px 45px rgba(0,0,0,.3)',
    },
    noCoverAnimation: {
      '&, &::before, &::after': { animation: 'none' },
    },
    cover: {
      objectFit: 'contain',
      cursor: 'pointer',
      display: 'block',
      width: '100%',
      height: '100%',
      backgroundColor: 'transparent',
      borderRadius: 5,
      transition: 'opacity 0.3s ease-in-out',
    },
    loveButton: {
      top: theme.spacing(-0.2),
      left: theme.spacing(0.5),
    },
    notes: {
      display: 'inline-block',
      marginTop: '1em',
      float: 'left',
      wordBreak: 'break-word',
      cursor: 'pointer',
    },
    recordName: {
      fontFamily: '"Newsreader", Georgia, serif',
      fontSize: 'clamp(28px, 4vw, 56px)',
      lineHeight: 1.08,
      overflowWrap: 'anywhere',
    },
    recordArtist: { color: theme.palette.primary.main, fontWeight: 600 },
    recordMeta: {
      color: theme.palette.text.secondary,
      fontSize: 13,
      marginTop: theme.spacing(1),
    },
    genreList: {
      marginTop: theme.spacing(0.5),
    },
    genreRow: {
      display: 'flex',
      alignItems: 'center',
      flexWrap: 'wrap',
      gap: theme.spacing(1),
      marginTop: theme.spacing(1),
    },
    externalLinks: {
      marginTop: theme.spacing(1.5),
    },
  }),
  {
    name: 'NDAlbumDetails',
  },
)

const useGetHandleGenreClick = (width) => {
  const [perPage] = useAlbumsPerPage(width)

  return (id) => {
    return `/album?filter={"genre_id":["${id}"]}&order=ASC&sort=name&perPage=${perPage}`
  }
}

const GenreChipField = withWidth()(({ width, ...rest }) => {
  const record = useRecordContext(rest)
  const genreLink = useGetHandleGenreClick(width)

  return (
    <Link to={genreLink(record.id)} onClick={(e) => e.stopPropagation()}>
      <ChipField
        source="name"
        // Workaround to force ChipField to be clickable
        onClick={() => {}}
      />
    </Link>
  )
})

const GenreList = () => {
  const classes = useStyles()
  return (
    <ArrayField className={classes.genreList} source={'genres'}>
      <SingleFieldList linkType={false}>
        <GenreChipField />
      </SingleFieldList>
    </ArrayField>
  )
}

export const Details = (props) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  const translate = useTranslate()
  const record = useRecordContext(props)
  const locale = useDateLocale()

  // Create an array of detail elements
  let details = []
  const addDetail = (obj) => {
    const id = details.length
    details.push(<span key={`detail-${record.id}-${id}`}>{obj}</span>)
  }

  // Calculate date related fields
  const yearRange = formatRange(record, 'year')
  const date = record.date ? formatFullDate(record.date, locale) : yearRange

  const originalDate = record.originalDate
    ? formatFullDate(record.originalDate, locale)
    : formatRange(record, 'originalYear')
  const releaseDate =
    record?.releaseDate && formatFullDate(record.releaseDate, locale)

  const dateToUse = originalDate || date
  const isOriginalDate = originalDate && dateToUse !== date
  const showDate = dateToUse && dateToUse !== releaseDate

  // Get label for the main date display
  const getDateLabel = () => {
    if (isXsmall) return '♫'
    if (isOriginalDate) return translate('resources.album.fields.originalDate')
    return null
  }

  // Get label for release date display
  const getReleaseDateLabel = () => {
    if (!isXsmall) return translate('resources.album.fields.releaseDate')
    if (showDate) return '○'
    return null
  }

  // Display dates with appropriate labels
  if (showDate) {
    addDetail(<>{[getDateLabel(), dateToUse].filter(Boolean).join('  ')}</>)
  }

  if (releaseDate) {
    addDetail(
      <>{[getReleaseDateLabel(), releaseDate].filter(Boolean).join('  ')}</>,
    )
  }
  addDetail(
    <>
      {record.songCount +
        ' ' +
        translate('resources.song.name', {
          smart_count: record.songCount,
        })}
    </>,
  )
  !isXsmall && addDetail(<DurationField source={'duration'} />)
  !isXsmall && addDetail(<SizeField source="size" />)

  // Return the details rendered with separators
  return <>{intersperse(details, ' · ')}</>
}

const AlbumDetails = (props) => {
  const record = useRecordContext(props)
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('lg'))
  const classes = useStyles()
  const [expanded, setExpanded] = useState(false)
  const [albumInfo, setAlbumInfo] = useState()
  const [isLightboxOpen, setLightboxOpen] = useState(false)

  let notes = albumInfo?.notes || record.notes

  if (notes) {
    notes += '..'
  }

  useEffect(() => {
    subsonic
      .getAlbumInfo(record.id)
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data.status === 'ok') {
          setAlbumInfo(data.albumInfo)
        }
      })
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.error('error on album page', e)
      })
  }, [record])

  const fullImageUrl = subsonic.getCoverArtUrl(record)

  return (
    <Card className={classes.root}>
      <div className={classes.cardContents}>
        <div
          className={clsx(
            classes.coverParent,
            !config.enableCoverAnimation && classes.noCoverAnimation,
          )}
        >
          <Artwork
            record={record}
            fit="contain"
            className={classes.cover}
            title={record.name}
            onClick={() => setLightboxOpen(true)}
          />
        </div>
        <div className={classes.details}>
          <CardContent className={classes.content}>
            <Typography
              variant={isDesktop ? 'h5' : 'h6'}
              className={classes.recordName}
            >
              {record.name}
              <LoveButton
                className={classes.loveButton}
                record={record}
                resource={'album'}
                size={isDesktop ? 'default' : 'small'}
                aria-label="love"
                color="primary"
              />
            </Typography>
            <Typography component={'h6'} className={classes.recordArtist}>
              {record?.tags?.['albumversion']}
            </Typography>
            <Typography component={'h6'} className={classes.recordArtist}>
              <ArtistLinkField record={record} />
            </Typography>
            <Typography component={'div'} className={classes.recordMeta}>
              <Details />
            </Typography>
            {config.enableStarRating && (
              <div>
                <RatingField
                  record={record}
                  resource={'album'}
                  size={isDesktop ? 'medium' : 'small'}
                />
              </div>
            )}
            <div className={classes.genreRow}>
              {isDesktop ? (
                <GenreList />
              ) : (
                record.genre && (
                  <Typography component={'span'}>{record.genre}</Typography>
                )
              )}
              <EditAlbumTags record={record} />
              <AudiobookCategoryButton album={record} />
            </div>
            {!isXsmall && (
              <Typography component={'div'} className={classes.recordMeta}>
                {config.enableExternalServices && (
                  <AlbumExternalLinks className={classes.externalLinks} />
                )}
              </Typography>
            )}
            {isDesktop && notes && (
              <Collapse
                collapsedHeight={'2.75em'}
                in={expanded}
                timeout={'auto'}
                className={classes.notes}
              >
                <Typography
                  variant={'body1'}
                  onClick={() => setExpanded(!expanded)}
                >
                  <span>
                    <SafeHTML>{notes}</SafeHTML>
                  </span>
                </Typography>
              </Collapse>
            )}
            {isDesktop && record['comment'] && (
              <CollapsibleComment record={record} />
            )}
          </CardContent>
        </div>
      </div>
      {!isDesktop && record['comment'] && (
        <CollapsibleComment record={record} />
      )}
      {!isDesktop && notes && (
        <div className={classes.notes}>
          <Collapse collapsedHeight={'1.5em'} in={expanded} timeout={'auto'}>
            <Typography
              variant={'body1'}
              onClick={() => setExpanded(!expanded)}
            >
              <span>
                <SafeHTML>{notes}</SafeHTML>
              </span>
            </Typography>
          </Collapse>
        </div>
      )}
      {isLightboxOpen && (
        <Lightbox
          imagePadding={50}
          animationDuration={200}
          imageTitle={record.name}
          mainSrc={fullImageUrl}
          onCloseRequest={() => setLightboxOpen(false)}
        />
      )}
    </Card>
  )
}

export default AlbumDetails
