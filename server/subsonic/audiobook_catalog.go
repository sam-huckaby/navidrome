package subsonic

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/filter"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
	"github.com/navidrome/navidrome/utils/slice"
)

const (
	unknownAudiobookAuthor  = "__audiobook_unknown_author__"
	uncategorizedAudiobooks = "__audiobook_uncategorized__"
)

func (api *Router) catalogBooks(r *http.Request) (model.Albums, error) {
	libraries, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, err
	}
	opts := filter.ApplyLibraryFilter(albumTypeFilter(filter.AlbumsByName(), true), libraries)
	return api.ds.Album().GetAll(r.Context(), opts)
}

func meaningfulAuthor(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.EqualFold(name, consts.UnknownArtist) &&
		!strings.EqualFold(name, consts.VariousArtists)
}

type authorCredit struct {
	id   string
	name string
}

// resolveBookAuthors picks an album artist when present; otherwise it votes over
// chapter artists. A typo on one chapter cannot turn a book into a compilation.
func resolveBookAuthors(books model.Albums, tracks model.MediaFiles) map[string]authorCredit {
	votes := make(map[string]map[string]int)
	credits := make(map[string]map[string]authorCredit)
	for _, track := range tracks {
		if track.Missing || track.ArtistID == "" || !meaningfulAuthor(track.Artist) {
			continue
		}
		if votes[track.AlbumID] == nil {
			votes[track.AlbumID] = make(map[string]int)
			credits[track.AlbumID] = make(map[string]authorCredit)
		}
		votes[track.AlbumID][track.ArtistID]++
		credits[track.AlbumID][track.ArtistID] = authorCredit{id: track.ArtistID, name: track.Artist}
	}
	result := make(map[string]authorCredit, len(books))
	for _, book := range books {
		if book.AlbumArtistID != "" && meaningfulAuthor(book.AlbumArtist) {
			result[book.ID] = authorCredit{id: book.AlbumArtistID, name: book.AlbumArtist}
			continue
		}
		best := authorCredit{id: unknownAudiobookAuthor, name: "Unknown Author"}
		bestCount := 0
		for id, count := range votes[book.ID] {
			candidate := credits[book.ID][id]
			if count > bestCount || (count == bestCount &&
				(strings.ToLower(candidate.name) < strings.ToLower(best.name) ||
					(strings.EqualFold(candidate.name, best.name) && id < best.id))) {
				best, bestCount = candidate, count
			}
		}
		result[book.ID] = best
	}
	return result
}

type audiobookAuthorGroup struct {
	author responses.ArtistID3
	books  model.Albums
}

func (api *Router) audiobookAuthorGroups(r *http.Request) (map[string]*audiobookAuthorGroup, error) {
	books, err := api.catalogBooks(r)
	if err != nil || len(books) == 0 {
		return map[string]*audiobookAuthorGroup{}, err
	}
	// Only books with missing/placeholder album-artist data need a chapter lookup.
	var fallbackIDs []string
	for _, book := range books {
		if book.AlbumArtistID == "" || !meaningfulAuthor(book.AlbumArtist) {
			fallbackIDs = append(fallbackIDs, book.ID)
		}
	}
	var tracks model.MediaFiles
	if len(fallbackIDs) > 0 {
		tracks, err = api.ds.MediaFile().GetAll(r.Context(), model.QueryOptions{
			Filters: And{Eq{"album_id": fallbackIDs}, Eq{"missing": false}},
		})
		if err != nil {
			return nil, err
		}
	}
	credits := resolveBookAuthors(books, tracks)
	groups := make(map[string]*audiobookAuthorGroup)
	var artistIDs []string
	for _, book := range books {
		credit := credits[book.ID]
		group := groups[credit.id]
		if group == nil {
			group = &audiobookAuthorGroup{author: responses.ArtistID3{Id: credit.id, Name: credit.name}}
			groups[credit.id] = group
			if credit.id != unknownAudiobookAuthor {
				artistIDs = append(artistIDs, credit.id)
			}
		}
		group.books = append(group.books, book)
	}
	if len(artistIDs) > 0 {
		artists, err := api.ds.Artist().GetAll(r.Context(), model.QueryOptions{Filters: Eq{"artist.id": artistIDs}})
		if err != nil {
			return nil, err
		}
		for _, artist := range artists {
			if group := groups[artist.ID]; group != nil {
				group.author = toArtistID3(r, artist)
			}
		}
	}
	for _, group := range groups {
		group.author.AlbumCount = int32(len(group.books))
		if group.author.CoverArt == "" {
			for _, book := range group.books {
				if !book.ImageAbsent {
					group.author.CoverArt = coverArtOrEmpty(book.CoverArtID(), book.ImageAbsent)
					break
				}
			}
		}
	}
	return groups, nil
}

func (api *Router) GetAudiobookAuthors(r *http.Request) (*responses.Subsonic, error) {
	groups, err := api.audiobookAuthorGroups(r)
	if err != nil {
		return nil, err
	}
	response := newResponse()
	response.AudiobookAuthors = &responses.AudiobookAuthors{}
	for _, group := range groups {
		response.AudiobookAuthors.Author = append(response.AudiobookAuthors.Author, group.author)
	}
	sort.Slice(response.AudiobookAuthors.Author, func(i, j int) bool {
		a, b := response.AudiobookAuthors.Author[i], response.AudiobookAuthors.Author[j]
		if strings.EqualFold(a.Name, b.Name) {
			return a.Id < b.Id
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return response, nil
}

func (api *Router) GetAudiobookAuthor(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	id, err := req.Params(r).String("id")
	if err != nil {
		return nil, err
	}
	groups, err := api.audiobookAuthorGroups(r)
	if err != nil {
		return nil, err
	}
	group := groups[id]
	if group == nil {
		return nil, newError(responses.ErrorDataNotFound, "Audiobook author not found")
	}
	ids := slice.Map(group.books, func(book model.Album) string { return book.ID })
	p := req.Params(r)
	opts := filter.AlbumsByName()
	opts.Filters = And{opts.Filters, Eq{"album.id": ids}, Eq{"album.is_audiobook": true}}
	opts.Offset = max(p.IntOr("offset", 0), 0)
	opts.Max = min(max(p.IntOr("size", 40), 1), 500)
	books, err := api.ds.Album().GetAll(r.Context(), opts)
	if err != nil {
		return nil, err
	}
	w.Header().Set("x-total-count", strconv.Itoa(len(ids)))
	response := newResponse()
	response.AudiobookAuthor = &responses.AudiobookAuthor{ArtistID3: group.author,
		Album: slice.MapWithArg(books, r.Context(), buildAlbumID3)}
	return response, nil
}

func bookGenres(book model.Album) []string {
	var genres []string
	seen := make(map[string]bool)
	for _, genre := range book.Genres {
		name := strings.TrimSpace(genre.Name)
		key := strings.ToLower(name)
		if name == "" || key == "audiobook" || seen[key] {
			continue
		}
		genres = append(genres, name)
		seen[key] = true
	}
	return genres
}

// Uncategorized is virtual: no usable subject-genre tag is attached to the book.
func uncategorizedBookFilter() Sqlizer {
	return Expr(`NOT EXISTS (SELECT 1 FROM album_tags at JOIN tag ON tag.id=at.tag_id
		WHERE at.album_id=album.id AND tag.tag_name='genre'
		AND trim(tag.tag_value)<>'' AND lower(trim(tag.tag_value))<>'audiobook')`)
}

func (api *Router) GetAudiobookGenres(r *http.Request) (*responses.Subsonic, error) {
	books, err := api.catalogBooks(r)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*responses.AudiobookGenre)
	for _, book := range books {
		genres := bookGenres(book)
		if len(genres) == 0 {
			genres = []string{uncategorizedAudiobooks}
		}
		for _, name := range genres {
			key := strings.ToLower(name)
			genre := byName[key]
			if genre == nil {
				label := name
				if name == uncategorizedAudiobooks {
					label = "Uncategorized"
				}
				genre = &responses.AudiobookGenre{Value: name, Name: label}
				byName[key] = genre
			}
			genre.AlbumCount++
			genre.SongCount += int32(book.SongCount)
		}
	}
	response := newResponse()
	response.AudiobookGenres = &responses.AudiobookGenres{}
	for _, genre := range byName {
		response.AudiobookGenres.Genre = append(response.AudiobookGenres.Genre, *genre)
	}
	slices.SortFunc(response.AudiobookGenres.Genre, func(a, b responses.AudiobookGenre) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return response, nil
}
