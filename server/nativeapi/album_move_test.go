package nativeapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	_ "github.com/navidrome/navidrome/adapters/gotaglib"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/conf/configtest"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/db"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/persistence"
	"github.com/navidrome/navidrome/scanner"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/utils/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.senan.xyz/taglib"
)

func TestMoveCopyVerification(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	dest := filepath.Join(root, "dest")
	if err := os.MkdirAll(filepath.Join(source, "CD1"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"CD1/01.mp3": bytes.Repeat([]byte("audio"), 10000), "cover.jpg": []byte("cover")} {
		if err := os.WriteFile(filepath.Join(source, name), data, 0640); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyMoveTree(t.Context(), source, dest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "CD1/01.mp3"), []byte("corrupt"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := verifyMoveTree(t.Context(), source, dest); err == nil {
		t.Fatal("corrupted copy passed verification")
	}
	if _, err := os.Stat(filepath.Join(source, "CD1/01.mp3")); err != nil {
		t.Fatal("source was removed", err)
	}
	if err := os.Symlink("cover.jpg", filepath.Join(source, "symlink")); err != nil {
		t.Fatal(err)
	}
	if err := copyMoveTree(t.Context(), source, filepath.Join(root, "unsafe")); err == nil {
		t.Fatal("symlink copy should fail")
	}
}

type failedMoveTransaction struct{ model.DataStore }

func (s failedMoveTransaction) WithTx(block func(model.DataStore) error, scope ...string) error {
	return s.DataStore.WithTx(func(tx model.DataStore) error {
		if err := block(tx); err != nil {
			return err
		}
		return errors.New("simulated catalog failure")
	}, scope...)
}

var _ = Describe("explicit album library moves", func() {
	for _, crossFilesystem := range []bool{false, true} {
		It(fmt.Sprintf("preserves tracks, progress and edited metadata through a destination scan and enforces new library access (cross filesystem: %t)", crossFilesystem), func() {
			DeferCleanup(configtest.SetupConfig())
			conf.Server.DevExternalScanner = false
			root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
			Expect(err).ToNot(HaveOccurred())
			from, to := filepath.Join(root, "source"), filepath.Join(root, "destination")
			Expect(os.MkdirAll(filepath.Join(from, "Author/Book/CD1"), 0755)).To(Succeed())
			Expect(os.Mkdir(to, 0755)).To(Succeed())
			fixture, err := os.ReadFile("tests/fixtures/test.mp3")
			Expect(err).ToNot(HaveOccurred())
			trackPath := filepath.Join(from, "Author/Book/CD1/01.mp3")
			Expect(os.WriteFile(trackPath, fixture, 0644)).To(Succeed())
			Expect(taglib.WriteTags(trackPath, map[string][]string{"TITLE": {"Chapter 1"}, "ALBUM": {"Move Book"}, "ARTIST": {"Author"}, "ALBUMARTIST": {"Author"}}, 0)).To(Succeed())
			trackCount := 1
			if !crossFilesystem {
				Expect(os.MkdirAll(filepath.Join(from, "Author/Book/CD2"), 0755)).To(Succeed())
				second := filepath.Join(from, "Author/Book/CD2/02.mp3")
				Expect(os.WriteFile(second, fixture, 0644)).To(Succeed())
				Expect(taglib.WriteTags(second, map[string][]string{"TITLE": {"Chapter 2"}, "ALBUM": {"Move Book"}, "ARTIST": {"Author"}, "ALBUMARTIST": {"Author"}}, 0)).To(Succeed())
				trackCount = 2
			}
			Expect(os.WriteFile(filepath.Join(from, "Author/Book/cover.jpg"), []byte("external cover"), 0644)).To(Succeed())
			conf.Server.DbPath = filepath.Join(root, "move-test.db") + "?_foreign_keys=on"
			conf.Server.MusicFolder = from
			ctx := GinkgoT().Context()
			singleton.DeleteInstance[*sql.DB]()
			closeDB := db.Init(ctx)
			DeferCleanup(func() { closeDB(); singleton.DeleteInstance[*sql.DB]() })
			ds := persistence.New(db.Db())
			src := model.Library{ID: 1, Name: "Family", Path: from}
			dst := model.Library{ID: 2, Name: "Grownup", Path: to}
			Expect(ds.Library().Put(ctx, &src)).To(Succeed())
			Expect(ds.Library().Put(ctx, &dst)).To(Succeed())
			ctx = request.WithUser(ctx, model.User{ID: "admin", IsAdmin: true, Libraries: []model.Library{src, dst}})
			scan := scanner.New(ctx, ds, events.NoopBroker(), playlists.NewPlaylists(ds, artwork.NewUploader(ds)), metrics.NewNoopInstance())
			_, err = scan.ScanAll(ctx, true)
			Expect(err).ToNot(HaveOccurred())
			tracks, err := ds.MediaFile().GetAll(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(tracks).To(HaveLen(trackCount))
			old := tracks[0]
			Expect(ds.Album().SetAudiobookOverride(ctx, old.AlbumID, true)).To(Succeed())
			tags := model.Tags{model.TagGenre: {"Private genre"}}
			Expect(ds.Album().SetTagOverrides(ctx, old.AlbumID, tags)).To(Succeed())
			user := model.User{ID: "move-reader", UserName: "move-reader", NewPassword: "test"}
			Expect(ds.User().Put(ctx, &user)).To(Succeed())
			admin := model.User{ID: "admin", UserName: "move-admin", IsAdmin: true, NewPassword: "test"}
			Expect(ds.User().Put(ctx, &admin)).To(Succeed())
			Expect(ds.Album().SetStar(ctx, true, old.AlbumID)).To(Succeed())
			playlist := model.Playlist{ID: "move-playlist", Name: "Move playlist", OwnerID: admin.ID}
			Expect(ds.Playlist().Put(ctx, &playlist)).To(Succeed())
			_, err = ds.Playlist().Tracks(ctx, playlist.ID, false).Add(ctx, []string{old.ID})
			Expect(err).ToNot(HaveOccurred())
			old.ChapterData = `{"chapters":[{"id":"ch1","title":"Embedded chapter","startMs":0,"endMs":1000}]}`
			Expect(ds.MediaFile().Put(ctx, &old)).To(Succeed())
			Expect(ds.Album().SaveAudiobookProgress(ctx, model.AudiobookProgress{AlbumID: old.AlbumID, UserID: user.ID, TrackID: old.ID, PositionMs: 12345})).To(Succeed())
			// Conflicts and unrelated files must be discovered before moving any bytes.
			Expect(os.MkdirAll(filepath.Join(to, "Author/Book"), 0755)).To(Succeed())
			_, err = prepareAlbumMove(ctx, ds, old.AlbumID, 2)
			Expect(err).To(MatchError(ContainSubstring("destination folder")))
			Expect(os.Remove(filepath.Join(to, "Author/Book"))).To(Succeed())
			Expect(os.WriteFile(filepath.Join(from, "Author/Book/unrelated.mp3"), []byte("other album"), 0644)).To(Succeed())
			_, err = prepareAlbumMove(ctx, ds, old.AlbumID, 2)
			Expect(err).To(MatchError(ContainSubstring("unrelated file")))
			Expect(os.Remove(filepath.Join(from, "Author/Book/unrelated.mp3"))).To(Succeed())
			plan, err := prepareAlbumMove(ctx, ds, old.AlbumID, 2)
			Expect(err).ToNot(HaveOccurred())
			Expect(plan.folder).To(Equal("Author/Book"))
			// A database failure must leave the source usable and remove the published copy.
			failedService := &albumMoveService{ctx: ctx, ds: failedMoveTransaction{ds}, scanner: scan, instance: "test"}
			if crossFilesystem {
				failedService.rename = func(string, string) error { return syscall.EXDEV }
			}
			failedRelease, locked := scanner.LockForMaintenance()
			Expect(locked).To(BeTrue())
			failedService.run(ctx, plan, albumMoveJob{ID: "failed-move", State: "moving"}, failedRelease)
			failedData, err := ds.Property().DefaultGet(ctx, "album-move:failed-move", "")
			Expect(err).ToNot(HaveOccurred())
			var failure albumMoveJob
			Expect(json.Unmarshal([]byte(failedData), &failure)).To(Succeed())
			Expect(failure.State).To(Equal("failed"))
			Expect(failure.Error).To(ContainSubstring("simulated catalog failure"))
			_, err = os.Stat(trackPath)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(plan.to)
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			unchanged, err := ds.MediaFile().Get(ctx, old.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(unchanged.LibraryID).To(Equal(src.ID))
			_, err = ds.Album().Get(ctx, plan.newAlbum.ID)
			Expect(errors.Is(err, model.ErrNotFound)).To(BeTrue())
			originalProgress, err := ds.Album().GetAudiobookProgress(ctx, old.AlbumID, user.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(originalProgress).ToNot(BeNil())
			release, ok := scanner.LockForMaintenance()
			Expect(ok).To(BeTrue())
			svc := &albumMoveService{ctx: ctx, ds: ds, scanner: scan, instance: "test"}
			if crossFilesystem {
				svc.rename = func(string, string) error { return syscall.EXDEV }
			}
			job := albumMoveJob{ID: "move-test", AlbumID: old.AlbumID, NewAlbumID: plan.newAlbum.ID, State: "moving", Instance: "test"}
			svc.run(ctx, plan, job, release)
			data, err := ds.Property().DefaultGet(ctx, "album-move:move-test", "")
			Expect(err).ToNot(HaveOccurred())
			var finished albumMoveJob
			Expect(json.Unmarshal([]byte(data), &finished)).To(Succeed())
			Expect(finished.State).To(Equal("complete"), finished.Error)
			Expect(finished.Warning).To(BeEmpty())
			moved, err := ds.MediaFile().Get(ctx, old.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(moved.LibraryID).To(Equal(2))
			Expect(moved.Path).To(Equal(old.Path))
			Expect(moved.CreatedAt).To(BeTemporally("~", old.CreatedAt))
			Expect(moved.AlbumID).ToNot(Equal(old.AlbumID))
			Expect(moved.ChapterData).To(Equal(old.ChapterData))
			album, err := ds.Album().Get(ctx, moved.AlbumID)
			Expect(err).ToNot(HaveOccurred())
			Expect(album.IsAudiobook).To(BeTrue())
			Expect(album.Starred).To(BeTrue())
			Expect(album.Genre).To(Equal("Private genre"))
			Expect(album.SongCount).To(Equal(trackCount))
			_, err = ds.Album().Get(ctx, old.AlbumID)
			Expect(errors.Is(err, model.ErrNotFound)).To(BeTrue())
			progress, err := ds.Album().GetAudiobookProgress(ctx, moved.AlbumID, user.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(progress).ToNot(BeNil())
			Expect(progress.PositionMs).To(Equal(int64(12345)))
			Expect(progress.TrackID).To(Equal(old.ID))
			playlistIDs, err := ds.Playlist().Tracks(ctx, playlist.ID, false).GetMediaFileIDs(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(playlistIDs).To(Equal([]string{old.ID}))
			_, err = os.Stat(filepath.Join(from, "Author/Book"))
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			cover, err := os.ReadFile(filepath.Join(to, "Author/Book/cover.jpg"))
			Expect(err).ToNot(HaveOccurred())
			Expect(string(cover)).To(Equal("external cover"))
			childCtx := request.WithUser(context.Background(), model.User{ID: user.ID, Libraries: []model.Library{src}})
			_, err = ds.MediaFile().Get(childCtx, old.ID)
			Expect(errors.Is(err, model.ErrNotFound)).To(BeTrue())
			_, err = ds.Album().Get(childCtx, moved.AlbumID)
			Expect(errors.Is(err, model.ErrNotFound)).To(BeTrue())
		})
	}
})
