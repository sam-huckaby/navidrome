package nativeapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/scanner"
)

type albumMoveJob struct {
	ID                   string `json:"id"`
	AlbumID              string `json:"albumId"`
	NewAlbumID           string `json:"newAlbumId,omitempty"`
	SourceLibraryID      int    `json:"sourceLibraryId"`
	DestinationLibraryID int    `json:"destinationLibraryId"`
	Folder               string `json:"folder"`
	State                string `json:"state"`
	Error                string `json:"error,omitempty"`
	Warning              string `json:"warning,omitempty"`
	Instance             string `json:"instance"`
}

type albumMoveService struct {
	ctx      context.Context
	scanner  model.Scanner
	ds       model.DataStore
	mu       sync.Mutex
	active   bool
	instance string
	// Overridden in tests to exercise cross-filesystem copies without a second mount.
	rename func(string, string) error
}

// ConfigureAlbumMoves binds moves to the server lifetime, not a browser request.
func (api *Router) ConfigureAlbumMoves(ctx context.Context, scan model.Scanner) {
	api.albumMoves = &albumMoveService{ctx: ctx, scanner: scan, ds: api.ds, instance: uuid.NewString()}
}

func (api *Router) startAlbumMove(w http.ResponseWriter, r *http.Request) {
	if api.albumMoves == nil {
		albumMoveError(w, "album moves are unavailable", http.StatusServiceUnavailable)
		return
	}
	var input struct {
		LibraryID int `json:"libraryId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || input.LibraryID <= 0 {
		albumMoveError(w, "libraryId must be a destination library", http.StatusBadRequest)
		return
	}
	s := api.albumMoves
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		albumMoveError(w, "another album move is running", http.StatusConflict)
		return
	}
	s.active = true
	s.mu.Unlock()
	release, ok := scanner.LockForMaintenance()
	if !ok {
		s.finish()
		albumMoveError(w, "a library scan or maintenance operation is running; retry when it finishes", http.StatusConflict)
		return
	}
	plan, err := prepareAlbumMove(r.Context(), s.ds, chi.URLParam(r, "id"), input.LibraryID)
	if err != nil {
		release()
		s.finish()
		status := http.StatusConflict
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		albumMoveError(w, err.Error(), status)
		return
	}
	job := albumMoveJob{ID: uuid.NewString(), AlbumID: plan.album.ID, NewAlbumID: plan.newAlbum.ID,
		SourceLibraryID: plan.source.ID, DestinationLibraryID: plan.destination.ID, Folder: plan.folder, State: "moving", Instance: s.instance}
	if err := s.persist(r.Context(), &job); err != nil {
		release()
		s.finish()
		albumMoveError(w, "could not record move job", http.StatusInternalServerError)
		return
	}
	ctx := request.AddValues(s.ctx, r.Context())
	go s.run(ctx, plan, job, release)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func (s *albumMoveService) finish() { s.mu.Lock(); s.active = false; s.mu.Unlock() }

func albumMoveError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func (s *albumMoveService) persist(ctx context.Context, j *albumMoveJob) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return s.ds.Property().Put(ctx, "album-move:"+j.ID, string(data))
}

func (api *Router) albumMoveStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := api.ds.Property().DefaultGet(r.Context(), "album-move:"+id, "")
	if err != nil {
		albumMoveError(w, "could not load move status", http.StatusInternalServerError)
		return
	}
	var job albumMoveJob
	if data == "" || json.Unmarshal([]byte(data), &job) != nil {
		http.NotFound(w, r)
		return
	}
	if job.State != "complete" && job.State != "failed" && (api.albumMoves == nil || job.Instance != api.albumMoves.instance) {
		job.State = "failed"
		job.Error = "Server restarted during the move. Inspect both library folders before retrying; any recovery copy is retained. Run a scan after recovery."
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

type albumMovePlan struct {
	album, newAlbum     model.Album
	source, destination model.Library
	folder, from, to    string
	tracks              model.MediaFiles
	directories         []string
}

func prepareAlbumMove(ctx context.Context, ds model.DataStore, id string, destID int) (*albumMovePlan, error) {
	album, err := ds.Album().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if album.LibraryID == destID {
		return nil, errors.New("album is already in that library")
	}
	source, err := ds.Library().Get(ctx, album.LibraryID)
	if err != nil {
		return nil, err
	}
	dest, err := ds.Library().Get(ctx, destID)
	if err != nil {
		return nil, err
	}
	for _, lib := range []*model.Library{source, dest} {
		if !filepath.IsAbs(lib.Path) {
			return nil, errors.New("moves require local library directories")
		}
		// Check every parent too, so symlinked or unavailable mount paths cannot redirect a move.
		if err := checkMoveDirectory(lib.Path); err != nil {
			return nil, fmt.Errorf("library %s: %w", lib.Name, err)
		}
	}
	if pathsOverlap(source.Path, dest.Path) {
		return nil, errors.New("source and destination libraries must not overlap")
	}
	tracks, err := ds.MediaFile().GetAll(ctx, model.QueryOptions{Filters: squirrel.Eq{"album_id": id}})
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, errors.New("album has no tracks")
	}
	var folder string
	for _, track := range tracks {
		if track.LibraryID != source.ID || track.Missing {
			return nil, errors.New("all album tracks must be present in one library")
		}
		if _, err := checkedBookFile(source.Path, filepath.FromSlash(track.Path)); err != nil {
			return nil, err
		}
		info, err := os.Lstat(filepath.Join(source.Path, filepath.FromSlash(track.Path)))
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("album contains unavailable tracks")
		}
		dir := path.Dir(track.Path)
		if folder == "" {
			folder = dir
		}
		for dir != folder && !strings.HasPrefix(dir, folder+"/") {
			folder = path.Dir(folder)
			if folder == "." {
				break
			}
		}
	}
	if discFolder.MatchString(path.Base(folder)) {
		folder = path.Dir(folder)
	}
	if folder == "." || !filepath.IsLocal(filepath.FromSlash(folder)) {
		return nil, errors.New("album must have a dedicated folder below the library root")
	}
	plan := &albumMovePlan{album: *album, source: *source, destination: *dest, folder: folder,
		from: filepath.Join(source.Path, filepath.FromSlash(folder)), to: filepath.Join(dest.Path, filepath.FromSlash(folder))}
	if err := checkMoveDirectory(plan.from); err != nil {
		return nil, err
	}
	// A shared album directory cannot be moved even if some catalogued tracks are missing.
	others, err := ds.MediaFile().GetAll(ctx, model.QueryOptions{Filters: squirrel.Eq{"library_id": source.ID}})
	if err != nil {
		return nil, err
	}
	for _, track := range others {
		if track.AlbumID != id && (path.Dir(track.Path) == folder || strings.HasPrefix(track.Path, folder+"/")) {
			return nil, errors.New("album folder is shared with another album")
		}
	}
	if _, err := os.Lstat(plan.to); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("destination folder already exists or cannot be checked")
	}
	if err := checkExistingMoveParents(filepath.Dir(plan.to)); err != nil {
		return nil, err
	}
	err = filepath.WalkDir(plan.from, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("album folder contains a symlink")
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(source.Path, file)
			plan.directories = append(plan.directories, filepath.ToSlash(rel))
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("album folder contains a non-regular file")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	store, err := storage.For(source.Path)
	if err != nil {
		return nil, err
	}
	fsys, err := store.FS()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(tracks))
	for i, t := range tracks {
		paths[i] = t.Path
	}
	infos, err := fsys.ReadTags(paths...)
	if err != nil {
		return nil, fmt.Errorf("reading track metadata: %w", err)
	}
	for _, old := range tracks {
		info, ok := infos[old.Path]
		if !ok {
			return nil, fmt.Errorf("could not read metadata for %s", old.Path)
		}
		moved := metadata.New(old.Path, info).ToMediaFile(*dest, model.FolderID(*dest, path.Dir(old.Path)))
		if plan.newAlbum.ID != "" && moved.AlbumID != plan.newAlbum.ID {
			return nil, errors.New("destination library grouping would split this album")
		}
		if plan.newAlbum.ID == "" {
			collision, err := ds.Album().Exists(ctx, moved.AlbumID)
			if err != nil {
				return nil, err
			}
			if collision {
				return nil, errors.New("destination grouping would merge this album with an existing album")
			}
			plan.newAlbum = *album
			plan.newAlbum.ID = moved.AlbumID
			plan.newAlbum.LibraryID = dest.ID
			plan.newAlbum.LibraryPath = dest.Path
			plan.newAlbum.LibraryName = dest.Name
			plan.newAlbum.FolderIDs = nil
		}
		// Keep track identity, file-derived metadata, chapters, history and original dates.
		moved.ID = old.ID
		moved.ChapterData = old.ChapterData
		moved.CreatedAt = old.CreatedAt
		moved.BirthTime = old.BirthTime
		plan.tracks = append(plan.tracks, moved)
		plan.newAlbum.FolderIDs = append(plan.newAlbum.FolderIDs, moved.FolderID)
	}
	return plan, nil
}

func pathsOverlap(a, b string) bool {
	if relative, err := filepath.Rel(a, b); err == nil && filepath.IsLocal(relative) {
		return true
	}
	relative, err := filepath.Rel(b, a)
	return err == nil && filepath.IsLocal(relative)
}
func checkMoveDirectory(dir string) error {
	for {
		info, err := os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("directory is unavailable: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("library paths must be real directories, not symlinks")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
func checkExistingMoveParents(dir string) error {
	for {
		_, err := os.Lstat(dir)
		if err == nil {
			return checkMoveDirectory(dir)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return err
		}
		dir = parent
	}
}

func (s *albumMoveService) run(ctx context.Context, p *albumMovePlan, j albumMoveJob, release func()) {
	defer s.finish()
	locked := true
	defer func() {
		if locked {
			release()
		}
	}()
	fail := func(err error) {
		j.State = "failed"
		j.Error = err.Error()
		if e := s.persist(context.WithoutCancel(ctx), &j); e != nil {
			log.Error(ctx, "Could not save album move failure", e)
		}
		log.Error(ctx, "Album move failed", "job", j.ID, err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			fail(fmt.Errorf("move interrupted unexpectedly; inspect source and destination before retrying: %v", recovered))
		}
	}()
	for _, root := range []string{p.source.Path, p.destination.Path} {
		if err := checkMoveDirectory(root); err != nil {
			fail(err)
			return
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.to), 0755); err != nil {
		fail(err)
		return
	}
	if err := checkMoveDirectory(filepath.Dir(p.to)); err != nil {
		fail(err)
		return
	}
	if _, err := os.Lstat(p.to); !errors.Is(err, os.ErrNotExist) {
		fail(errors.New("destination appeared during move"))
		return
	}
	copied := false
	rename := s.rename
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(p.from, p.to); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			fail(err)
			return
		}
		stage := filepath.Join(filepath.Dir(p.to), ".album-move-"+j.ID)
		if err := copyMoveTree(ctx, p.from, stage); err != nil {
			_ = os.RemoveAll(stage)
			fail(err)
			return
		}
		if _, err := os.Lstat(p.to); !errors.Is(err, os.ErrNotExist) {
			_ = os.RemoveAll(stage)
			fail(errors.New("destination appeared during copy"))
			return
		}
		if err := os.Rename(stage, p.to); err != nil {
			fail(fmt.Errorf("could not publish copied album; recovery copy is at %s: %w", stage, err))
			return
		}
		copied = true
	}
	j.State = "updating"
	if err := s.persist(ctx, &j); err != nil {
		fail(errors.Join(err, rollbackMove(p, copied)))
		return
	}
	err := s.ds.WithTx(func(tx model.DataStore) error {
		if err := tx.Tag().Add(ctx, p.destination.ID, p.newAlbum.Tags.FlattenAll()...); err != nil {
			return err
		}
		for _, rel := range p.directories {
			if err := tx.Folder().Put(ctx, model.NewFolder(p.destination, rel)); err != nil {
				return err
			}
		}
		if err := tx.Album().Put(ctx, &p.newAlbum); err != nil {
			return err
		}
		for i := range p.tracks {
			if err := tx.Tag().Add(ctx, p.destination.ID, p.tracks[i].Tags.FlattenAll()...); err != nil {
				return err
			}
			if err := tx.MediaFile().Put(ctx, &p.tracks[i]); err != nil {
				return err
			}
			for _, artist := range p.tracks[i].Participants.AllArtists() {
				if err := tx.Library().AddArtist(ctx, p.destination.ID, artist.ID); err != nil {
					return err
				}
			}
		}
		return tx.Album().TransferMoveMetadata(ctx, p.album.ID, p.newAlbum.ID)
	}, "move album library")
	if err != nil {
		rollbackErr := rollbackMove(p, copied)
		fail(errors.Join(err, rollbackErr))
		return
	}
	// The catalog now points to the destination; never roll back files past this point.
	if copied {
		if err := verifyMoveTree(ctx, p.from, p.to); err != nil {
			j.Warning = "Move catalog updated, but source changed during copying; source files retained: " + err.Error()
		} else if err := os.RemoveAll(p.from); err != nil {
			j.Warning = "Destination is ready but source cleanup failed: " + err.Error()
		}
	}
	j.State = "scanning"
	_ = s.persist(ctx, &j)
	release()
	locked = false
	// Both sides are scanned in one run, including the source parent after its album directory disappears.
	targets := []model.ScanTarget{{LibraryID: p.source.ID, FolderPath: path.Dir(p.folder)}, {LibraryID: p.destination.ID, FolderPath: path.Dir(p.folder)}}
	for {
		_, err = s.scanner.ScanFolders(ctx, false, targets)
		if !errors.Is(err, model.ErrAlreadyScanning) {
			break
		}
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(time.Second):
			continue
		}
		break
	}
	if err != nil {
		j.Warning = strings.TrimSpace(j.Warning + " Catalog move completed; scan failed: " + err.Error())
	}
	j.State = "complete"
	if err := s.persist(context.WithoutCancel(ctx), &j); err != nil {
		log.Error(ctx, "Could not save completed album move", err)
	}
}

func rollbackMove(p *albumMovePlan, copied bool) error {
	if copied {
		// Never discard the recovery copy if the source drive disappeared or
		// changed after copying and before the database transaction failed.
		if err := verifyMoveTree(context.Background(), p.from, p.to); err != nil {
			return fmt.Errorf("source could not be verified; recovery copy retained at %s: %w", p.to, err)
		}
		return os.RemoveAll(p.to)
	}
	if _, err := os.Lstat(p.from); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("source path reappeared; recovery copy retained at %s", p.to)
	}
	return os.Rename(p.to, p.from)
}

func fileDigest(ctx context.Context, file string) ([32]byte, error) {
	var digest [32]byte
	f, err := os.Open(file)
	if err != nil {
		return digest, err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		n, e := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return digest, e
		}
	}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func copyMoveTree(ctx context.Context, from, to string) error {
	if err := os.Mkdir(to, 0755); err != nil {
		return err
	}
	err := filepath.WalkDir(from, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source became a symlink")
		}
		rel, _ := filepath.Rel(from, file)
		dest := filepath.Join(to, rel)
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Mkdir(dest, 0755)
		}
		if !info.Mode().IsRegular() {
			return errors.New("source contains a non-regular file")
		}
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		syncErr := out.Sync()
		closeErr := out.Close()
		if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
			return err
		}
		return os.Chtimes(dest, info.ModTime(), info.ModTime())
	})
	if err != nil {
		return err
	}
	return verifyMoveTree(ctx, from, to)
}

func verifyMoveTree(ctx context.Context, from, to string) error {
	seen := map[string]bool{}
	err := filepath.WalkDir(from, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source became a symlink")
		}
		rel, err := filepath.Rel(from, file)
		if err != nil {
			return err
		}
		seen[rel] = true
		info, err := os.Lstat(filepath.Join(to, rel))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != entry.IsDir() {
			return fmt.Errorf("destination type changed: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("source is not a regular file")
		}
		a, err := fileDigest(ctx, file)
		if err != nil {
			return err
		}
		b, err := fileDigest(ctx, filepath.Join(to, rel))
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("verification failed: %s", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return filepath.WalkDir(to, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(to, file)
		if err != nil {
			return err
		}
		if !seen[rel] || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("directory contents changed: %s", rel)
		}
		return nil
	})
}
