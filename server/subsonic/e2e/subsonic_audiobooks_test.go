package e2e

import (
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func audiobookXMLRequest(endpoint string, params ...string) *httptest.ResponseRecorder {
	r := buildReq(adminUser, endpoint, params...)
	query := r.URL.Query()
	query.Set("f", "xml")
	r.URL.RawQuery = query.Encode()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

var _ = Describe("Audiobook endpoints", func() {
	var book, otherBook model.Album
	var chapterID string

	BeforeEach(func() {
		setupTestDB()
		albums, err := ds.Album().GetAll(ctx)
		Expect(err).ToNot(HaveOccurred())
		for _, album := range albums {
			switch album.Name {
			case "Abbey Road":
				book = album
			case "Help!":
				otherBook = album
			}
		}
		Expect(book.ID).ToNot(BeEmpty())
		Expect(otherBook.ID).ToNot(BeEmpty())
		Expect(ds.Album().SetAudiobookOverride(ctx, book.ID, true)).To(Succeed())
		Expect(ds.Album().SetAudiobookOverride(ctx, otherBook.ID, true)).To(Succeed())
		chapters, err := ds.MediaFile().GetAll(ctx)
		Expect(err).ToNot(HaveOccurred())
		for _, chapter := range chapters {
			if chapter.AlbumID == book.ID {
				chapterID = chapter.ID
				break
			}
		}
		Expect(chapterID).ToNot(BeEmpty())
	})

	It("lists only books with pagination, while music listings and searches exclude them", func() {
		page := doRawReq("getAudiobooks", "size", "1", "offset", "0")
		Expect(page.Header().Get("x-total-count")).To(Equal("2"))
		Expect(parseJSONResponse(page).Audiobooks.Album).To(HaveLen(1))
		Expect(doReq("getAudiobooks", "size", "1", "offset", "1").Audiobooks.Album).To(HaveLen(1))
		Expect(doReq("getAudiobooks", "musicFolderId", "999").Status).To(Equal(responses.StatusFailed))

		albums := doReq("getAlbumList2", "type", "alphabeticalByName", "size", "500").AlbumList2.Album
		for _, album := range albums {
			Expect(album.Id).ToNot(BeElementOf(book.ID, otherBook.ID))
		}
		bookSearch := doReq("searchAudiobooks", "query", "Abbey", "size", "10")
		Expect(bookSearch.Audiobooks.Album).To(HaveLen(1))
		Expect(bookSearch.Audiobooks.Album[0].Id).To(Equal(book.ID))
		musicSearch := doReq("search3", "query", "Abbey", "artistCount", "0", "albumCount", "10", "songCount", "0")
		Expect(musicSearch.SearchResult3.Album).To(BeEmpty())
		chapterSearch := doReq("search3", "query", "Come Together", "artistCount", "0", "albumCount", "0", "songCount", "10")
		Expect(chapterSearch.SearchResult3.Song).To(BeEmpty())
		for _, song := range doReq("getRandomSongs", "size", "500").RandomSongs.Songs {
			Expect(song.AlbumId).ToNot(BeElementOf(book.ID, otherBook.ID))
		}
		for _, song := range doReq("getSongsByGenre", "genre", "Rock", "count", "500").SongsByGenre.Songs {
			Expect(song.AlbumId).ToNot(BeElementOf(book.ID, otherBook.ID))
		}
		artist := doReq("getArtist", "id", book.AlbumArtistID)
		for _, album := range artist.ArtistWithAlbumsID3.Album {
			Expect(album.Id).ToNot(BeElementOf(book.ID, otherBook.ID))
		}

		xml := audiobookXMLRequest("getAudiobooks")
		Expect(xml.Code).To(Equal(http.StatusOK))
		Expect(xml.Body.String()).To(ContainSubstring("<audiobooks>"))
	})

	It("returns ordered, streamable chapters and rejects non-books", func() {
		result := doReq("getAudiobook", "id", book.ID)
		Expect(result.Status).To(Equal(responses.StatusOK))
		Expect(result.Audiobook.Id).To(Equal(book.ID))
		Expect(result.Audiobook.Song).To(HaveLen(2))
		Expect(result.Audiobook.Song[0].Track).To(Equal(int32(1)))
		Expect(result.Audiobook.Song[1].Track).To(Equal(int32(2)))
		Expect(result.Audiobook.Song[0].Id).ToNot(BeEmpty())
		Expect(result.Audiobook.CoverArt).ToNot(BeEmpty())
		Expect(doReq("getAudiobook", "id", "not-a-book").Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(doReq("getAudiobook", "id", "").Error.Code).To(Equal(responses.ErrorMissingParameter))
	})

	It("shares per-user progress with the existing audiobook store and validates chapters", func() {
		reader := createUser("audiobook-reader", "audiobook-reader", "Audiobook Reader", false)
		initial := doReqWithUser(reader, "getAudiobookProgress", "id", book.ID).AudiobookProgress
		Expect(initial.TrackID).To(BeEmpty())
		Expect(initial.PositionMs).To(BeZero())
		Expect(doReqWithUser(reader, "saveAudiobookProgress", "id", book.ID, "trackId", chapterID,
			"positionMs", "45000", "completed", "true").Status).To(Equal(responses.StatusOK))
		position := doReqWithUser(reader, "getAudiobookProgress", "id", book.ID).AudiobookProgress
		Expect(position.TrackID).To(Equal(chapterID))
		Expect(position.PositionMs).To(Equal(int64(45000)))
		Expect(position.Completed).To(BeTrue())
		stored, err := ds.Album().GetAudiobookProgress(ctx, book.ID, reader.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(stored.PositionMs).To(Equal(int64(45000)))
		Expect(doReq("getAudiobookProgress", "id", book.ID).AudiobookProgress.TrackID).To(BeEmpty())
		Expect(doReqWithUser(reader, "saveAudiobookProgress", "id", otherBook.ID, "trackId", chapterID,
			"positionMs", "100").Status).To(Equal(responses.StatusFailed))
		Expect(doReqWithUser(reader, "saveAudiobookProgress", "id", book.ID, "trackId", chapterID,
			"positionMs", "-1").Status).To(Equal(responses.StatusFailed))
		Expect(doReqWithUser(reader, "saveAudiobookProgress", "id", book.ID, "trackId", chapterID,
			"positionMs", strconv.FormatInt(1<<50, 10)).Status).To(Equal(responses.StatusFailed))
		Expect(doReqWithUser(reader, "getAudiobookProgress", "id", "unknown").Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(audiobookXMLRequest("getAudiobookProgress", "id", book.ID).Body.String()).To(ContainSubstring("<audiobookProgress"))
	})

	It("does not expose books or progress from inaccessible libraries", func() {
		reader := createUser("no-book-library", "no-book-library", "Restricted Reader", false)
		Expect(ds.User().SetUserLibraries(ctx, reader.ID, nil)).To(Succeed())
		Expect(doReqWithUser(reader, "getAudiobooks").Audiobooks.Album).To(BeEmpty())
		Expect(doReqWithUser(reader, "searchAudiobooks", "query", "Abbey").Audiobooks.Album).To(BeEmpty())
		Expect(doReqWithUser(reader, "getAudiobook", "id", book.ID).Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(doReqWithUser(reader, "getAudiobookProgress", "id", book.ID).Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(doReqWithUser(reader, "saveAudiobookProgress", "id", book.ID, "trackId", chapterID,
			"positionMs", "10").Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(doReqWithUser(reader, "getAudiobookAuthors").AudiobookAuthors.Author).To(BeEmpty())
		Expect(doReqWithUser(reader, "getAudiobookGenres").AudiobookGenres.Genre).To(BeEmpty())
		Expect(doReqWithUser(reader, "getAudiobookAuthor", "id", book.AlbumArtistID).Error.Code).To(Equal(responses.ErrorDataNotFound))
	})

	It("provides author rows and paginated audiobook-only author details", func() {
		authors := doReq("getAudiobookAuthors").AudiobookAuthors.Author
		Expect(authors).To(HaveLen(1))
		Expect(authors[0].Id).To(Equal(book.AlbumArtistID))
		Expect(authors[0].AlbumCount).To(Equal(int32(2)))
		Expect(authors[0].CoverArt).ToNot(BeEmpty())
		page := doRawReq("getAudiobookAuthor", "id", authors[0].Id, "size", "1", "offset", "1")
		Expect(page.Header().Get("x-total-count")).To(Equal("2"))
		Expect(parseJSONResponse(page).AudiobookAuthor.Album).To(HaveLen(1))
		Expect(doReq("getAudiobookAuthor", "id", "missing").Error.Code).To(Equal(responses.ErrorDataNotFound))
		Expect(audiobookXMLRequest("getAudiobookAuthors").Body.String()).To(ContainSubstring("<audiobookAuthors>"))
		Expect(audiobookXMLRequest("getAudiobookAuthor", "id", authors[0].Id).Body.String()).To(ContainSubstring("<audiobookAuthor"))
	})

	It("counts only book genres and lists books by genre", func() {
		genres := doReq("getAudiobookGenres").AudiobookGenres.Genre
		Expect(genres).To(ContainElement(responses.AudiobookGenre{Value: "Rock", Name: "Rock", AlbumCount: 2, SongCount: 3}))
		Expect(doReq("getAudiobooks", "type", "byGenre", "genre", "Rock").Audiobooks.Album).To(HaveLen(2))
		Expect(audiobookXMLRequest("getAudiobookGenres").Body.String()).To(ContainSubstring("<audiobookGenres>"))
	})

	It("groups untagged and audiobook-only books under a navigable Uncategorized genre", func() {
		Expect(ds.Tag().Add(ctx, book.LibraryID, model.NewTag(model.TagGenre, "Audiobook"))).To(Succeed())
		book.Tags = model.Tags{model.TagGenre: {"Audiobook"}}
		otherBook.Tags = model.Tags{}
		book.IsAudiobook = true
		otherBook.IsAudiobook = true
		Expect(ds.Album().Put(ctx, &book)).To(Succeed())
		Expect(ds.Album().Put(ctx, &otherBook)).To(Succeed())
		genres := doReq("getAudiobookGenres").AudiobookGenres.Genre
		Expect(genres).To(Equal([]responses.AudiobookGenre{{
			Value: "__audiobook_uncategorized__", Name: "Uncategorized", AlbumCount: 2, SongCount: 3,
		}}))
		Expect(doReq("getAudiobooks", "type", "byGenre", "genre", "__audiobook_uncategorized__").Audiobooks.Album).To(HaveLen(2))
		Expect(doReq("getAudiobooks", "type", "byGenre", "genre", "Audiobook").Audiobooks.Album).To(BeEmpty())
	})

	It("keeps a subject genre when a book also carries the generic Audiobook tag", func() {
		Expect(ds.Tag().Add(ctx, book.LibraryID,
			model.NewTag(model.TagGenre, "Audiobook"), model.NewTag(model.TagGenre, "Fantasy"))).To(Succeed())
		book.Tags = model.Tags{model.TagGenre: {"Audiobook", "Fantasy"}}
		book.IsAudiobook = true
		Expect(ds.Album().Put(ctx, &book)).To(Succeed())
		genres := doReq("getAudiobookGenres").AudiobookGenres.Genre
		Expect(genres).To(ContainElement(responses.AudiobookGenre{
			Value: "Fantasy", Name: "Fantasy", AlbumCount: 1, SongCount: 2,
		}))
		Expect(doReq("getAudiobooks", "type", "byGenre", "genre", "Fantasy").Audiobooks.Album).To(HaveLen(1))
		for _, album := range doReq("getAudiobooks", "type", "byGenre", "genre", "__audiobook_uncategorized__").Audiobooks.Album {
			Expect(album.Id).ToNot(Equal(book.ID))
		}
	})

	It("exposes an Unknown Author row for uncredited books", func() {
		uncredited := model.Album{ID: "uncredited-book", LibraryID: book.LibraryID,
			Name: "Uncredited Book", IsAudiobook: true}
		Expect(ds.Album().Put(ctx, &uncredited)).To(Succeed())
		authors := doReq("getAudiobookAuthors").AudiobookAuthors.Author
		Expect(authors).To(ContainElement(SatisfyAll(
			HaveField("Id", "__audiobook_unknown_author__"),
			HaveField("Name", "Unknown Author"),
			HaveField("AlbumCount", int32(1)),
		)))
		books := doReq("getAudiobookAuthor", "id", "__audiobook_unknown_author__").AudiobookAuthor.Album
		Expect(books).To(HaveLen(1))
		Expect(books[0].Id).To(Equal(uncredited.ID))
	})
})
