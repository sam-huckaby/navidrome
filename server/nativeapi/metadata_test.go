package nativeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/external"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.senan.xyz/taglib"
)

type fakeProvider struct {
	external.Provider
	mu     sync.Mutex
	called []string
}

func (f *fakeProvider) RefreshInfo(_ context.Context, kind model.Kind, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = append(f.called, kind.Prefix()+"/"+id)
	return nil
}

func (f *fakeProvider) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.called)
}

var _ = Describe("Metadata API", func() {
	var ctx context.Context
	var ds *tests.MockDataStore
	var artRepo *tests.MockArtworkRepo
	var queueRepo *tests.MockArtworkQueueRepo
	var albumRepo *tests.MockAlbumRepo
	var provider *fakeProvider
	var router http.Handler
	var adminToken, userToken string

	BeforeEach(func() {
		ctx = GinkgoT().Context()
		DeferCleanup(configtest.SetupConfig())
		conf.Server.EnableSharing = false
		artRepo = tests.CreateMockArtworkRepo()
		queueRepo = tests.CreateMockArtworkQueueRepo()
		albumRepo = tests.CreateMockAlbumRepo()
		artistRepo := tests.CreateMockArtistRepo()
		playlistRepo := tests.CreateMockPlaylistRepo()
		Expect(albumRepo.Put(ctx, &model.Album{ID: "al-1", Name: "Kid A"})).To(Succeed())
		Expect(artistRepo.Put(ctx, &model.Artist{ID: "ar-1", Name: "Radiohead"})).To(Succeed())
		Expect(playlistRepo.Put(ctx, &model.Playlist{ID: "pl-1", Name: "My Playlist"})).To(Succeed())
		ds = &tests.MockDataStore{
			MockedArtwork:      artRepo,
			MockedArtworkQueue: queueRepo,
			MockedAlbum:        albumRepo,
			MockedArtist:       artistRepo,
			MockedPlaylist:     playlistRepo,
		}
		auth.Init(ds)
		provider = &fakeProvider{}
		nativeRouter := New(ds, nil, playlists.NewPlaylists(ds, nil), nil, tests.NewMockLibraryService(), tests.NewMockUserService(), nil, nil, nil, provider, nil)
		router = server.JWTVerifier(nativeRouter)

		adminUser := model.User{ID: "admin-1", UserName: "admin", IsAdmin: true, NewPassword: "adminpass"}
		regularUser := model.User{ID: "user-1", UserName: "regular", IsAdmin: false, NewPassword: "userpass"}
		Expect(ds.User().Put(ctx, &adminUser)).To(Succeed())
		Expect(ds.User().Put(ctx, &regularUser)).To(Succeed())

		var err error
		adminToken, err = auth.CreateToken(&adminUser)
		Expect(err).ToNot(HaveOccurred())
		userToken, err = auth.CreateToken(&regularUser)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("album tag overrides", func() {
		BeforeEach(func() {
			files := tests.CreateMockMediaFileRepo()
			files.SetData(model.MediaFiles{{ID: "mf-1", AlbumID: "al-1", Album: "Kid A", Title: "Everything In Its Right Place", LibraryID: 1,
				Tags: model.Tags{model.TagGenre: {"Rock"}}}})
			ds.MockedMediaFile = files
			ds.MockedTag = &tests.MockTagRepo{}
		})

		It("allows admins to change album genres without changing song tags and to restore the originals", func() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/album/al-1/tags", bytes.NewBufferString(`{"genre":["Jazz","Electronic"],"mood":["Calm"]}`), adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			album, err := albumRepo.Get(ctx, "al-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(album.Tags[model.TagGenre]).To(Equal([]string{"Jazz", "Electronic"}))
			Expect(album.Tags[model.TagMood]).To(Equal([]string{"Calm"}))
			file, err := ds.MediaFile().GetWithParticipants(ctx, "mf-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(file.Tags[model.TagGenre]).To(Equal([]string{"Rock"}))

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/album/al-1/tags", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			Expect(album.Tags[model.TagGenre]).To(Equal([]string{"Rock"}))
			Expect(album.Tags[model.TagMood]).To(BeEmpty())
		})

		It("rejects disallowed tags and denies edits by non-admins", func() {
			for _, body := range []string{`{"artist":["Someone"]}`, `{"genre":[""]}`, `{"genre":"Rock"}`} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/album/al-1/tags", bytes.NewBufferString(body), adminToken))
				Expect(w.Code).To(Equal(http.StatusBadRequest), body)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/album/al-1/tags", bytes.NewBufferString(`{"genre":["Jazz"]}`), userToken))
			Expect(w.Code).To(Equal(http.StatusForbidden))
		})
	})

	Describe("audiobook classification and listening position", func() {
		BeforeEach(func() {
			files := tests.CreateMockMediaFileRepo()
			files.SetData(model.MediaFiles{{ID: "chapter-1", AlbumID: "al-1", Title: "Chapter 1", Duration: 120}})
			ds.MockedMediaFile = files
		})

		It("lets admins classify an album while readers independently save chapter progress", func() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/album/al-1/audiobook", bytes.NewBufferString(`{"isAudiobook":true}`), userToken))
			Expect(w.Code).To(Equal(http.StatusForbidden))

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/album/al-1/audiobook", bytes.NewBufferString(`{"isAudiobook":true}`), adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			album, err := albumRepo.Get(ctx, "al-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(album.IsAudiobook).To(BeTrue())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/audiobook/al-1/progress", bytes.NewBufferString(`{"trackId":"chapter-1","positionMs":45000}`), userToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("GET", "/audiobook/al-1/progress", nil, userToken))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(`"positionMs":45000`))
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("GET", "/audiobook/al-1/progress", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring(`"trackId":""`))

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/audiobook/al-1/progress", bytes.NewBufferString(`{"trackId":"wrong","positionMs":4000}`), userToken))
			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("audiobook deletion", func() {
		var libraryPath string
		var files *tests.MockMediaFileRepo
		BeforeEach(func() {
			libraryPath = GinkgoT().TempDir()
			libs := &tests.MockLibraryRepo{}
			libs.SetData(model.Libraries{{ID: 1, Name: "Books", Path: libraryPath}})
			ds.MockedLibrary = libs
			Expect(albumRepo.Put(ctx, &model.Album{ID: "al-1", Name: "Book", LibraryID: 1, IsAudiobook: true})).To(Succeed())
			files = tests.CreateMockMediaFileRepo()
			ds.MockedMediaFile = files
		})

		It("restricts deletion to admins and removes an exclusive book's chapters, cover and catalogue entries", func() {
			folder := filepath.Join(libraryPath, "Author", "Book")
			Expect(os.MkdirAll(folder, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(folder, "01.mp3"), []byte("chapter"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(folder, "cover.jpg"), []byte("cover"), 0644)).To(Succeed())
			files.SetData(model.MediaFiles{{ID: "chapter", AlbumID: "al-1", LibraryID: 1, Path: "Author/Book/01.mp3"}})

			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, userToken))
			Expect(w.Code).To(Equal(http.StatusForbidden))
			_, err := os.Stat(filepath.Join(folder, "01.mp3"))
			Expect(err).ToNot(HaveOccurred())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			_, err = os.Stat(filepath.Join(folder, "01.mp3"))
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			_, err = os.Stat(filepath.Join(folder, "cover.jpg"))
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			_, err = files.Get(ctx, "chapter")
			Expect(errors.Is(err, model.ErrNotFound)).To(BeTrue())
			Expect(ds.GCCalled).To(BeTrue())
		})

		It("leaves shared-folder artwork and other chapters intact", func() {
			folder := filepath.Join(libraryPath, "Shared")
			Expect(os.Mkdir(folder, 0755)).To(Succeed())
			for _, name := range []string{"book.mp3", "music.mp3", "cover.jpg"} {
				Expect(os.WriteFile(filepath.Join(folder, name), []byte(name), 0644)).To(Succeed())
			}
			files.SetData(model.MediaFiles{
				{ID: "book", AlbumID: "al-1", LibraryID: 1, Path: "Shared/book.mp3"},
				{ID: "music", AlbumID: "other", LibraryID: 1, Path: "Shared/music.mp3"},
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			for _, name := range []string{"music.mp3", "cover.jpg"} {
				_, err := os.Stat(filepath.Join(folder, name))
				Expect(err).ToNot(HaveOccurred())
			}
			_, err := files.Get(ctx, "music")
			Expect(err).ToNot(HaveOccurred())
		})

		It("removes a cover in the book root when chapters live in disc folders", func() {
			folder := filepath.Join(libraryPath, "Author", "Book")
			for _, disc := range []string{"CD1", "CD2"} {
				Expect(os.MkdirAll(filepath.Join(folder, disc), 0755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(folder, disc, "01.mp3"), []byte("chapter"), 0644)).To(Succeed())
			}
			Expect(os.WriteFile(filepath.Join(folder, "cover.jpg"), []byte("cover"), 0644)).To(Succeed())
			files.SetData(model.MediaFiles{
				{ID: "one", AlbumID: "al-1", LibraryID: 1, Path: "Author/Book/CD1/01.mp3"},
				{ID: "two", AlbumID: "al-1", LibraryID: 1, Path: "Author/Book/CD2/01.mp3"},
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			_, err := os.Stat(folder)
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})

		It("removes the parent cover when only one disc folder has chapters", func() {
			folder := filepath.Join(libraryPath, "Author", "Book")
			Expect(os.MkdirAll(filepath.Join(folder, "CD1"), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(folder, "CD1", "01.mp3"), []byte("chapter"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(folder, "cover.jpg"), []byte("cover"), 0644)).To(Succeed())
			files.SetData(model.MediaFiles{{ID: "chapter", AlbumID: "al-1", LibraryID: 1, Path: "Author/Book/CD1/01.mp3"}})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			_, err := os.Stat(folder)
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		})

		It("does not delete albums that are not audiobooks", func() {
			Expect(albumRepo.Put(ctx, &model.Album{ID: "al-1", LibraryID: 1})).To(Succeed())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("rejects symlinked paths before deleting any chapter", func() {
			outside := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(outside, "chapter.mp3"), []byte("outside"), 0644)).To(Succeed())
			Expect(os.Symlink(outside, filepath.Join(libraryPath, "link"))).To(Succeed())
			files.SetData(model.MediaFiles{{ID: "chapter", AlbumID: "al-1", LibraryID: 1, Path: "link/chapter.mp3"}})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("DELETE", "/audiobook/al-1", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusConflict))
			_, err := os.Stat(filepath.Join(outside, "chapter.mp3"))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("music folder uploads", func() {
		var libraryPath string
		BeforeEach(func() {
			base := GinkgoT().TempDir()
			libraryPath = filepath.Join(base, "library")
			Expect(os.Mkdir(libraryPath, 0755)).To(Succeed())
			conf.Server.DataFolder = conf.NewDir(filepath.Join(base, "data"))
			libs := &tests.MockLibraryRepo{}
			libs.SetData(model.Libraries{{ID: 1, Name: "Music", Path: libraryPath}})
			ds.MockedLibrary = libs
		})

		It("allows admin uploads, rejects traversal, imports audio and skips duplicates", func() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("GET", "/music-upload/status", nil, userToken))
			Expect(w.Code).To(Equal(http.StatusForbidden))
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("GET", "/music-upload/status", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(ContainSubstring("fpcalcInstalled"))

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("POST", "/music-upload", bytes.NewBufferString(`{"libraryId":1}`), userToken))
			Expect(w.Code).To(Equal(http.StatusForbidden))

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("POST", "/music-upload", bytes.NewBufferString(`{"libraryId":1}`), adminToken))
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			var created struct {
				ID string `json:"id"`
			}
			Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
			Expect(created.ID).ToNot(BeEmpty())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/music-upload/"+created.ID+"/file?path=..%2Foutside.mp3", bytes.NewBufferString("bad"), adminToken))
			Expect(w.Code).To(Equal(http.StatusBadRequest))

			audio, err := os.ReadFile("tests/fixtures/test.mp3")
			Expect(err).ToNot(HaveOccurred())
			rel := "Selected Album/test.mp3"
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/music-upload/"+created.ID+"/file?path="+url.QueryEscape(rel), bytes.NewBuffer(audio), adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())
			cover, err := os.ReadFile("tests/fixtures/artist/an-album/cover.jpg")
			Expect(err).ToNot(HaveOccurred())
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/music-upload/"+created.ID+"/file?path="+url.QueryEscape("Selected Album/cover.jpg"), bytes.NewBuffer(cover), adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent), w.Body.String())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("GET", "/music-upload/"+created.ID+"/preview", nil, adminToken))
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(w.Body.String()).To(ContainSubstring(rel))

			payload := `{"tracks":[{"path":"Selected Album/test.mp3","title":"Song","artist":"Alice","album":"My Album","albumArtist":"Alice","isAudiobook":true}]}`
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("POST", "/music-upload/"+created.ID+"/import", bytes.NewBufferString(payload), adminToken))
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(w.Body.String()).To(ContainSubstring("test.mp3"))
			bookTrack := filepath.Join(libraryPath, "Alice", "My Album", "test.mp3")
			_, err = os.Stat(bookTrack)
			Expect(err).ToNot(HaveOccurred())
			tags, err := taglib.ReadTags(bookTrack)
			Expect(err).ToNot(HaveOccurred())
			Expect(firstUploadTag(tags, "RELEASETYPE")).To(Equal("audiobook"))
			_, err = os.Stat(filepath.Join(libraryPath, "Alice", "My Album", "cover.jpg"))
			Expect(err).ToNot(HaveOccurred())

			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("POST", "/music-upload", bytes.NewBufferString(`{"libraryId":1}`), adminToken))
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(json.Unmarshal(w.Body.Bytes(), &created)).To(Succeed())
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("PUT", "/music-upload/"+created.ID+"/file?path="+url.QueryEscape(rel), bytes.NewBuffer(audio), adminToken))
			Expect(w.Code).To(Equal(http.StatusNoContent))
			w = httptest.NewRecorder()
			router.ServeHTTP(w, createAuthenticatedRequest("POST", "/music-upload/"+created.ID+"/import", bytes.NewBufferString(payload), adminToken))
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(w.Body.String()).To(ContainSubstring(`"skipped":["Selected Album/test.mp3"]`))
		})
	})

	Describe("POST /api/metadata/{kind}/{id}/refresh", func() {
		It("clears state and enqueues a Bump for admins", func() {
			Expect(artRepo.PutItemArtwork(ctx, &model.ItemArtwork{
				ItemKind: "al", ItemID: "al-1", Hash: "oldhash", Source: "external",
			})).To(Succeed())

			req := createAuthenticatedRequest("POST", "/metadata/al/al-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNoContent))

			_, err := artRepo.GetItemArtwork(ctx, model.KindAlbumArtwork, "al-1", model.ImageTypePrimary)
			Expect(err).To(MatchError(model.ErrNotFound))

			queued, err := queueRepo.DequeueBatch(ctx, 1000)
			Expect(err).ToNot(HaveOccurred())
			Expect(queued).To(ContainElement(SatisfyAll(
				HaveField("ItemKind", "al"),
				HaveField("ItemID", "al-1"),
				HaveField("Priority", model.ArtworkPriorityBump),
			)))
		})

		It("returns 400 for an invalid kind", func() {
			req := createAuthenticatedRequest("POST", "/metadata/xx/id-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})

		It("denies access to regular users", func() {
			req := createAuthenticatedRequest("POST", "/metadata/al/al-1/refresh", nil, userToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusForbidden))
		})

		It("denies access without authentication", func() {
			req := createUnauthenticatedRequest("POST", "/metadata/al/al-1/refresh", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))
		})

		It("triggers an external info refresh for albums", func() {
			req := createAuthenticatedRequest("POST", "/metadata/al/al-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNoContent))
			Eventually(provider.calls).Should(ContainElement("al/al-1"))
		})

		It("triggers an external info refresh for artists", func() {
			req := createAuthenticatedRequest("POST", "/metadata/ar/ar-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNoContent))
			Eventually(provider.calls).Should(ContainElement("ar/ar-1"))
		})

		It("skips the external info refresh for kinds without external info", func() {
			req := createAuthenticatedRequest("POST", "/metadata/pl/pl-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNoContent))
			Consistently(provider.calls).ShouldNot(ContainElement("pl/pl-1"))
		})

		It("returns 404 for an unknown id", func() {
			req := createAuthenticatedRequest("POST", "/metadata/al/nope/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("returns 500 when the lookup fails for a reason other than not-found", func() {
			albumRepo.SetError(true)

			req := createAuthenticatedRequest("POST", "/metadata/al/al-1/refresh", nil, adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusInternalServerError))
		})
	})
})
