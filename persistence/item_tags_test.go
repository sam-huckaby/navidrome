package persistence

import (
	"context"

	"github.com/deluan/rest"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pocketbase/dbx"
)

var _ = Describe("item genre tag indexes", func() {
	var conn *dbx.DB
	var mr model.MediaFileRepository
	var ar model.AlbumRepository
	var rock, jazz model.Tag
	var ctx context.Context

	BeforeEach(func() {
		ctx = request.WithUser(GinkgoT().Context(), model.User{ID: "userid"})
		conn = GetDBXBuilder()
		mr = NewMediaFileRepository(conn)
		ar = NewAlbumRepository(conn)
		// Test-only genre values, so they can't collide with the golden fixtures.
		rock = model.NewTag(model.TagGenre, "GenreIdxRock")
		jazz = model.NewTag(model.TagGenre, "GenreIdxJazz")
		// The join tables FK to tag(id); the scanner adds tags before saving items.
		Expect(NewTagRepository(conn).Add(ctx, 1, rock, jazz)).To(Succeed())
		// The suite shares one golden DB with no per-test restore, so undo the rows we add
		// (media_file/album deletes cascade to the *_tags join rows; tag deletes cascade too).
		DeferCleanup(func() {
			_, _ = conn.NewQuery("DELETE FROM media_file WHERE id LIKE 'mf-%'").Execute()
			_, _ = conn.NewQuery("DELETE FROM album WHERE id LIKE 'al-%'").Execute()
			_, _ = conn.NewQuery("DELETE FROM tag WHERE id={:r} OR id={:j}").
				Bind(dbx.Params{"r": rock.ID, "j": jazz.ID}).Execute()
		})
	})

	tagIDsFor := func(table, col, id string) []string {
		var rows []struct {
			TagID string `db:"tag_id"`
		}
		err := conn.NewQuery("SELECT tag_id FROM " + table + " WHERE " + col + "={:id}").
			Bind(dbx.Params{"id": id}).All(&rows)
		Expect(err).ToNot(HaveOccurred())
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.TagID
		}
		return ids
	}

	Describe("media files", func() {
		It("writes a media_file_tags row for each genre when the track is saved", func() {
			mf := model.MediaFile{ID: "mf-g1", LibraryID: 1, Path: "/m/g1.mp3", Title: "G1",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue, jazz.TagValue}}}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			Expect(tagIDsFor("media_file_tags", "media_file_id", "mf-g1")).To(ConsistOf(rock.ID, jazz.ID))
		})

		It("replaces the rows when the genres change", func() {
			mf := model.MediaFile{ID: "mf-g2", LibraryID: 1, Path: "/m/g2.mp3", Title: "G2",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue}}}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			mf.Tags = model.Tags{model.TagGenre: []string{jazz.TagValue}}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			Expect(tagIDsFor("media_file_tags", "media_file_id", "mf-g2")).To(ConsistOf(jazz.ID))
		})

		It("clears the rows when all genres are removed", func() {
			mf := model.MediaFile{ID: "mf-g3", LibraryID: 1, Path: "/m/g3.mp3", Title: "G3",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue}}}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			mf.Tags = model.Tags{}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			Expect(tagIDsFor("media_file_tags", "media_file_id", "mf-g3")).To(BeEmpty())
		})
	})

	Describe("albums", func() {
		It("writes an album_tags row for each genre when the album is saved", func() {
			al := model.Album{ID: "al-g1", LibraryID: 1, Name: "AG1",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue, jazz.TagValue}}}
			Expect(ar.Put(ctx, &al)).To(Succeed())
			Expect(tagIDsFor("album_tags", "album_id", "al-g1")).To(ConsistOf(rock.ID, jazz.ID))
		})
	})

	It("classifies audiobooks from tags, filters music lists, and keeps a manual override", func() {
		book := model.Album{ID: "al-book", LibraryID: 1, Name: "A Story", Tags: model.Tags{model.TagReleaseType: {"audiobook"}}}
		Expect(ar.ApplyAudiobookOverride(ctx, &book)).To(Succeed())
		Expect(book.IsAudiobook).To(BeTrue())
		Expect(ar.Put(ctx, &book)).To(Succeed())
		stored, err := ar.Get(ctx, book.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(stored.IsAudiobook).To(BeTrue())
		chapter := model.MediaFile{ID: "mf-book", AlbumID: book.ID, LibraryID: 1, Path: "/m/chapter.mp3", Title: "Chapter 1"}
		Expect(mr.Put(ctx, &chapter)).To(Succeed())

		books, err := ar.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"audiobook": true}})
		Expect(err).ToNot(HaveOccurred())
		Expect(books).To(ContainElement(HaveField("ID", book.ID)))
		books, err = ar.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"audiobook": "true"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(books).To(ContainElement(HaveField("ID", book.ID)))
		music, err := ar.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"audiobook": false}})
		Expect(err).ToNot(HaveOccurred())
		Expect(music).ToNot(ContainElement(HaveField("ID", book.ID)))
		musicTracks, err := mr.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"audiobook": "false"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(musicTracks).ToNot(ContainElement(HaveField("ID", chapter.ID)))

		Expect(ar.SetAudiobookOverride(ctx, book.ID, false)).To(Succeed())
		Expect(ar.ApplyAudiobookOverride(ctx, &book)).To(Succeed())
		Expect(book.IsAudiobook).To(BeFalse())
		Expect(ar.Put(ctx, &book)).To(Succeed())
		musicTracks, err = mr.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"audiobook": false}})
		Expect(err).ToNot(HaveOccurred())
		Expect(musicTracks).To(ContainElement(HaveField("ID", chapter.ID)))
	})

	It("hides audiobook-only artists from the music artist list", func() {
		artist := model.Artist{ID: "ar-bookonly", Name: "Book Narrator", OrderArtistName: "book narrator"}
		artists := NewArtistRepository(conn)
		Expect(artists.Put(ctx, &artist)).To(Succeed())
		Expect(NewLibraryRepository(conn).AddArtist(ctx, 1, artist.ID)).To(Succeed())
		DeferCleanup(func() {
			_, _ = conn.NewQuery("DELETE FROM album WHERE id='al-bookonly'").Execute()
			_, _ = conn.NewQuery("DELETE FROM artist WHERE id='ar-bookonly'").Execute()
		})
		book := model.Album{ID: "al-bookonly", LibraryID: 1, Name: "A Story", IsAudiobook: true,
			Participants: model.Participants{model.RoleAlbumArtist: {{Artist: artist}}}}
		Expect(ar.Put(ctx, &book)).To(Succeed())
		musicArtists, err := artists.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"music_only": "true"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(musicArtists).ToNot(ContainElement(HaveField("ID", artist.ID)))
		book.IsAudiobook = false
		Expect(ar.Put(ctx, &book)).To(Succeed())
		musicArtists, err = artists.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"music_only": true}})
		Expect(err).ToNot(HaveOccurred())
		Expect(musicArtists).To(ContainElement(HaveField("ID", artist.ID)))
	})

	It("saves the last chapter and position separately for each audiobook listener", func() {
		album := model.Album{ID: "al-progress-book", LibraryID: 1, Name: "Long Book", IsAudiobook: true}
		Expect(ar.Put(ctx, &album)).To(Succeed())
		user := model.User{ID: "book-progress-user", UserName: "book-progress-user", NewPassword: "test"}
		Expect(NewUserRepository(conn).Put(ctx, &user)).To(Succeed())
		DeferCleanup(func() {
			_, _ = conn.NewQuery("DELETE FROM user WHERE id='book-progress-user'").Execute()
		})
		progress := model.AudiobookProgress{AlbumID: album.ID, UserID: user.ID, TrackID: "chapter-2", PositionMs: 123456}
		Expect(ar.SaveAudiobookProgress(ctx, progress)).To(Succeed())
		got, err := ar.GetAudiobookProgress(ctx, album.ID, user.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.TrackID).To(Equal("chapter-2"))
		Expect(got.PositionMs).To(Equal(int64(123456)))
		other, err := ar.GetAudiobookProgress(ctx, album.ID, "userid")
		Expect(err).ToNot(HaveOccurred())
		Expect(other).To(BeNil())
		progress.Completed = true
		Expect(ar.SaveAudiobookProgress(ctx, progress)).To(Succeed())
		got, err = ar.GetAudiobookProgress(ctx, album.ID, user.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Completed).To(BeTrue())
	})

	It("persists album-only genre overrides, updates indexes, and restores file genres", func() {
		albumID := "al-override"
		fileID := "mf-override"
		mf := model.MediaFile{ID: fileID, AlbumID: albumID, Album: "Override", LibraryID: 1,
			Path: "/m/override.mp3", Title: "Track", Tags: model.Tags{model.TagGenre: {rock.TagValue}}}
		Expect(mr.Put(ctx, &mf)).To(Succeed())
		album := model.Album{ID: albumID, LibraryID: 1, Name: "Override", Tags: model.Tags{model.TagGenre: {rock.TagValue}}}
		Expect(ar.Put(ctx, &album)).To(Succeed())

		Expect(ar.SetTagOverrides(ctx, albumID, model.Tags{model.TagGenre: {jazz.TagValue}})).To(Succeed())
		album.Tags = model.Tags{model.TagGenre: {rock.TagValue}}
		Expect(ar.ApplyTagOverrides(ctx, &album)).To(Succeed())
		Expect(ar.Put(ctx, &album)).To(Succeed())
		Expect(tagIDsFor("album_tags", "album_id", albumID)).To(Equal([]string{jazz.ID}))
		Expect(tagIDsFor("media_file_tags", "media_file_id", fileID)).To(Equal([]string{rock.ID}))
		got, err := ar.Get(ctx, albumID)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Genre).To(Equal(jazz.TagValue))

		// A rescan recomputes from the file, then overlays the saved values.
		rebuilt := model.MediaFiles{mf}.ToAlbum()
		Expect(ar.ApplyTagOverrides(ctx, &rebuilt)).To(Succeed())
		Expect(rebuilt.Tags[model.TagGenre]).To(Equal([]string{jazz.TagValue}))
		Expect(ar.SetTagOverrides(ctx, albumID, model.Tags{})).To(Succeed())
		rebuilt = model.MediaFiles{mf}.ToAlbum()
		Expect(ar.ApplyTagOverrides(ctx, &rebuilt)).To(Succeed())
		Expect(rebuilt.Tags[model.TagGenre]).To(Equal([]string{rock.TagValue}))
		Expect(ar.Put(ctx, &rebuilt)).To(Succeed())
		Expect(tagIDsFor("album_tags", "album_id", albumID)).To(Equal([]string{rock.ID}))
	})

	// The native (REST) API filters by genre_id; it must resolve through the join table too.
	Describe("native genre_id filter", func() {
		It("filters media files by genre_id", func() {
			mf := model.MediaFile{ID: "mf-nat1", LibraryID: 1, Path: "/m/nat1.mp3", Title: "Nat1",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue}}}
			Expect(mr.Put(ctx, &mf)).To(Succeed())
			res, err := mr.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"genre_id": rock.ID}})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(ContainElement(HaveField("ID", "mf-nat1")))
		})

		It("filters albums by genre_id", func() {
			al := model.Album{ID: "al-nat1", LibraryID: 1, Name: "ANat1",
				Tags: model.Tags{model.TagGenre: []string{rock.TagValue}}}
			Expect(ar.Put(ctx, &al)).To(Succeed())
			res, err := ar.ReadAll(ctx, rest.QueryOptions{Filters: map[string]any{"genre_id": rock.ID}})
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(ContainElement(HaveField("ID", "al-nat1")))
		})
	})
})
