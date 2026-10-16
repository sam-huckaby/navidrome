package subsonic

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/deluan/sanitize"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/filter"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
	"github.com/navidrome/navidrome/utils/slice"
)

// Keep chapter filtering at the query level so pagination never includes audiobook tracks in music results.
func musicTrackFilter() Sqlizer {
	return Eq{"coalesce((select album.is_audiobook from album where album.id=media_file.album_id),0)": 0}
}

func albumTypeFilter(opts filter.Options, audiobook bool) filter.Options {
	condition := Eq{"album.is_audiobook": audiobook}
	if opts.Filters == nil {
		opts.Filters = condition
	} else {
		opts.Filters = And{opts.Filters, condition}
	}
	return opts
}

func (api *Router) GetAudiobook(r *http.Request) (*responses.Subsonic, error) {
	id, err := req.Params(r).String("id")
	if err != nil {
		return nil, err
	}
	album, err := api.audiobook(r, id)
	if err != nil {
		return nil, err
	}
	chapters, err := api.ds.MediaFile().GetAll(r.Context(), filter.SongsByAlbum(id))
	if err != nil {
		return nil, err
	}
	sort.SliceStable(chapters, func(i, j int) bool {
		a, b := chapters[i], chapters[j]
		if a.DiscNumber != b.DiscNumber {
			return a.DiscNumber < b.DiscNumber
		}
		if a.TrackNumber != b.TrackNumber {
			return a.TrackNumber < b.TrackNumber
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	response := newResponse()
	response.Audiobook = api.buildAlbum(r.Context(), album, chapters)
	return response, nil
}

func (api *Router) SearchAudiobooks(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	query, err := p.String("query")
	if err != nil {
		return nil, err
	}
	libraryIDs, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, err
	}
	opts := model.QueryOptions{Max: min(max(p.IntOr("size", 20), 1), 500), Offset: max(p.IntOr("offset", 0), 0),
		Filters: Eq{"album.is_audiobook": true}}
	opts = filter.ApplyLibraryFilter(opts, libraryIDs)
	albums, err := api.ds.Album().Search(r.Context(), sanitize.Accents(strings.ToLower(strings.TrimSuffix(query, "*"))), opts)
	if err != nil {
		return nil, err
	}
	response := newResponse()
	response.Audiobooks = &responses.AlbumList2{Album: slice.MapWithArg(albums, r.Context(), buildAlbumID3)}
	return response, nil
}

func (api *Router) audiobook(r *http.Request, id string) (*model.Album, error) {
	album, err := api.ds.Album().Get(r.Context(), id)
	if errors.Is(err, model.ErrNotFound) {
		return nil, newError(responses.ErrorDataNotFound, "Audiobook not found")
	}
	if err != nil {
		return nil, err
	}
	if !album.IsAudiobook || album.Missing {
		return nil, newError(responses.ErrorDataNotFound, "Audiobook not found")
	}
	return album, nil
}

func (api *Router) GetAudiobookProgress(r *http.Request) (*responses.Subsonic, error) {
	id, err := req.Params(r).String("id")
	if err != nil {
		return nil, err
	}
	if _, err := api.audiobook(r, id); err != nil {
		return nil, err
	}
	user, _ := request.UserFrom(r.Context())
	progress, err := api.ds.Album().GetAudiobookProgress(r.Context(), id, user.ID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		progress = &model.AudiobookProgress{AlbumID: id}
	}
	response := newResponse()
	response.AudiobookProgress = &responses.AudiobookProgress{
		AlbumID: progress.AlbumID, TrackID: progress.TrackID, PositionMs: progress.PositionMs,
		Completed: progress.Completed, UpdatedAt: progress.UpdatedAt,
	}
	return response, nil
}

func (api *Router) SaveAudiobookProgress(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	if _, err := api.audiobook(r, id); err != nil {
		return nil, err
	}
	trackID, err := p.String("trackId")
	if err != nil {
		return nil, err
	}
	position, err := p.Int64("positionMs")
	if err != nil {
		return nil, err
	}
	if position < 0 {
		return nil, fmt.Errorf("%w: positionMs must be non-negative", req.ErrInvalidParam)
	}
	completed := p.BoolOr("completed", false)
	track, err := api.ds.MediaFile().Get(r.Context(), trackID)
	if errors.Is(err, model.ErrNotFound) || (err == nil && (track.AlbumID != id || track.Missing)) {
		return nil, newError(responses.ErrorDataNotFound, "Chapter not found in audiobook")
	}
	if err != nil {
		return nil, err
	}
	if track.Duration > 0 && position > int64(track.Duration*1000)+5000 {
		return nil, fmt.Errorf("%w: positionMs exceeds chapter duration", req.ErrInvalidParam)
	}
	user, _ := request.UserFrom(r.Context())
	if err := api.ds.Album().SaveAudiobookProgress(r.Context(), model.AudiobookProgress{
		AlbumID: id, UserID: user.ID, TrackID: trackID, PositionMs: position,
		Completed: completed, UpdatedAt: time.Now(),
	}); err != nil {
		return nil, err
	}
	return newResponse(), nil
}
