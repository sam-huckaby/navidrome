package nativeapi

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
)

// checkedBookFile confines a stored library-relative path to regular files inside
// a local library. Never follow symlinks, including in parent directories.
func checkedBookFile(root, rel string) (string, error) {
	if !filepath.IsAbs(root) || rel == "" || filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
		return "", errors.New("invalid audiobook path")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("library directory is unavailable")
	}
	current := root
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return filepath.Join(root, rel), nil // missing files can still have catalogue entries
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return "", errors.New("audiobook path is not a regular file")
		}
	}
	return current, nil
}

// bookCompanions identifies sidecar files only when the entire book directory
// contains no other files (or symlinks). Shared folders are never cleared.
func bookCompanions(root string, tracks []string) ([]string, []string, error) {
	if len(tracks) == 0 {
		return nil, nil, nil
	}
	bookRoot := filepath.Dir(tracks[0])
	for _, track := range tracks[1:] {
		dir := filepath.Dir(track)
		for dir != bookRoot && !strings.HasPrefix(dir, bookRoot+string(filepath.Separator)) {
			parent := filepath.Dir(bookRoot)
			if parent == bookRoot {
				return nil, nil, nil
			}
			bookRoot = parent
		}
	}
	if discFolder.MatchString(filepath.Base(bookRoot)) {
		bookRoot = filepath.Dir(bookRoot)
	}
	if bookRoot == root {
		return nil, nil, nil
	}
	known := make(map[string]bool, len(tracks))
	for _, file := range tracks {
		known[file] = true
	}
	var companions, dirs []string
	shared := false
	err := filepath.WalkDir(bookRoot, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			shared = true
			return nil
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
			return nil
		}
		if !entry.Type().IsRegular() {
			shared = true
			return nil
		}
		if known[name] {
			return nil
		}
		if !companionExtensions[strings.ToLower(filepath.Ext(name))] {
			shared = true
			return nil
		}
		companions = append(companions, name)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if shared {
		return nil, nil, nil
	}
	for _, file := range companions {
		rel, _ := filepath.Rel(root, file)
		if _, err := checkedBookFile(root, rel); err != nil {
			return nil, nil, err
		}
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(b) - len(a) })
	return companions, dirs, nil
}

func (api *Router) deleteAudiobook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	album, err := api.ds.Album().Get(ctx, id)
	if errors.Is(err, model.ErrNotFound) || err == nil && !album.IsAudiobook {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "could not load audiobook", http.StatusInternalServerError)
		return
	}
	lib, err := api.ds.Library().Get(ctx, album.LibraryID)
	if err != nil {
		http.Error(w, "library is unavailable", http.StatusConflict)
		return
	}
	files, err := api.ds.MediaFile().GetAll(ctx, model.QueryOptions{Filters: squirrel.Eq{"album_id": id}})
	if err != nil {
		http.Error(w, "could not load chapters", http.StatusInternalServerError)
		return
	}
	var tracks []model.MediaFile
	var paths []string
	for _, file := range files {
		if file.AlbumID != id {
			continue
		}
		if file.LibraryID != lib.ID {
			http.Error(w, "audiobook spans multiple libraries", http.StatusConflict)
			return
		}
		fullPath, err := checkedBookFile(lib.Path, filepath.FromSlash(file.Path))
		if err != nil {
			http.Error(w, "unsafe audiobook path", http.StatusConflict)
			return
		}
		tracks = append(tracks, file)
		paths = append(paths, fullPath)
	}
	if len(tracks) == 0 {
		http.Error(w, "audiobook has no chapters", http.StatusConflict)
		return
	}
	companions, dirs, err := bookCompanions(lib.Path, paths)
	if err != nil {
		http.Error(w, "could not inspect audiobook folder", http.StatusConflict)
		return
	}
	for _, file := range append(paths, companions...) {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			http.Error(w, "could not remove audiobook files", http.StatusConflict)
			return
		}
	}
	for _, dir := range dirs {
		_ = os.Remove(dir) // remove empty book folders only; never a shared or library root
	}
	if err := api.ds.WithTx(func(tx model.DataStore) error {
		for _, track := range tracks {
			if err := tx.MediaFile().Delete(ctx, track.ID); err != nil {
				return err
			}
		}
		return nil
	}, "delete audiobook chapters"); err != nil {
		http.Error(w, "could not remove audiobook from catalogue", http.StatusInternalServerError)
		return
	}
	if err := api.ds.GC(ctx, lib.ID); err != nil {
		http.Error(w, "could not clean audiobook catalogue", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
