package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"go.senan.xyz/taglib"
	"golang.org/x/time/rate"
)

const maxMusicFileSize int64 = 2 << 30

var acoustIDLimiter = rate.NewLimiter(3, 1)
var musicBrainzLimiter = rate.NewLimiter(1, 1)

var musicExtensions = map[string]bool{
	".mp3": true, ".flac": true, ".m4a": true, ".aac": true, ".ogg": true,
	".opus": true, ".wav": true, ".aiff": true, ".aif": true, ".wma": true,
	".ape": true, ".wv": true, ".dsf": true,
}

var companionExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".lrc": true, ".elrc": true, ".ttml": true, ".srt": true,
	".txt": true, ".cue": true, ".yaml": true, ".yml": true,
}

type uploadSession struct {
	LibraryID int `json:"libraryId"`
}

type uploadedTrack struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AlbumArtist string `json:"albumArtist"`
	NeedsMatch  bool   `json:"needsMatch"`
	IsAudiobook bool   `json:"isAudiobook"`
}

func (api *Router) addMusicUploadRoutes(r chi.Router) {
	r.Get("/music-upload/status", func(w http.ResponseWriter, r *http.Request) {
		_, err := exec.LookPath("fpcalc")
		writeUploadJSON(w, map[string]any{"fpcalcInstalled": err == nil, "matchingConfigured": conf.Server.AcoustIDKey != ""})
	})
	r.Post("/music-upload", api.createMusicUpload)
	r.Route("/music-upload/{id}", func(r chi.Router) {
		r.Put("/file", api.receiveMusicFile)
		r.Get("/preview", api.previewMusicUpload)
		r.Post("/match", api.matchMusicTrack)
		r.Post("/import", api.importMusicUpload)
		r.Delete("/", api.cancelMusicUpload)
	})
}

func uploadRoot() string { return filepath.Join(conf.Server.DataFolder.String(), "uploads") }

func pruneMusicUploads() {
	entries, err := os.ReadDir(uploadRoot())
	if err != nil {
		return
	}
	for _, entry := range entries {
		dir, err := sessionDir(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, "session.json"))
		if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(dir)
		}
	}
}

func sessionDir(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return "", errors.New("invalid upload session")
	}
	return filepath.Join(uploadRoot(), id), nil
}

func readUploadSession(id string) (uploadSession, string, error) {
	dir, err := sessionDir(id)
	if err != nil {
		return uploadSession{}, "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return uploadSession{}, "", err
	}
	var session uploadSession
	err = json.Unmarshal(b, &session)
	return session, dir, err
}

func validUploadPath(raw string) (string, error) {
	if raw == "" || len(raw) > 1024 || strings.ContainsAny(raw, "\\\x00") || path.IsAbs(raw) {
		return "", errors.New("invalid relative path")
	}
	for _, part := range strings.Split(raw, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return "", errors.New("invalid relative path")
		}
	}
	if path.Clean(raw) != raw {
		return "", errors.New("invalid relative path")
	}
	return raw, nil
}

func writeUploadJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (api *Router) createMusicUpload(w http.ResponseWriter, r *http.Request) {
	var input uploadSession
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || input.LibraryID < 1 {
		http.Error(w, "invalid library", http.StatusBadRequest)
		return
	}
	lib, err := api.ds.Library().Get(r.Context(), input.LibraryID)
	if err != nil {
		http.Error(w, "library not found", http.StatusNotFound)
		return
	}
	if !filepath.IsAbs(lib.Path) || strings.Contains(lib.Path, "://") {
		http.Error(w, "only local libraries support uploads", http.StatusBadRequest)
		return
	}
	if info, err := os.Lstat(lib.Path); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "library directory is unavailable", http.StatusBadRequest)
		return
	}
	check, err := os.CreateTemp(lib.Path, ".music-upload-check-*")
	if err != nil {
		http.Error(w, "library directory is not writable", http.StatusConflict)
		return
	}
	_ = check.Close()
	_ = os.Remove(check.Name())
	if err := os.MkdirAll(uploadRoot(), 0700); err != nil {
		http.Error(w, "cannot prepare upload storage", http.StatusInternalServerError)
		return
	}
	pruneMusicUploads()
	id := uuid.NewString()
	dir, _ := sessionDir(id)
	if err := os.Mkdir(dir, 0700); err != nil {
		http.Error(w, "cannot create upload", http.StatusInternalServerError)
		return
	}
	b, _ := json.Marshal(input)
	if err := os.WriteFile(filepath.Join(dir, "session.json"), b, 0600); err != nil {
		_ = os.RemoveAll(dir)
		http.Error(w, "cannot create upload", http.StatusInternalServerError)
		return
	}
	writeUploadJSON(w, map[string]string{"id": id})
}

func (api *Router) receiveMusicFile(w http.ResponseWriter, r *http.Request) {
	_, dir, err := readUploadSession(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel, err := validUploadPath(r.URL.Query().Get("path"))
	if err != nil || (!musicExtensions[strings.ToLower(path.Ext(rel))] && !companionExtensions[strings.ToLower(path.Ext(rel))]) {
		http.Error(w, "unsupported file or path", http.StatusBadRequest)
		return
	}
	dest := filepath.Join(dir, "files", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil { //nolint:gosec // the session ID and each path component were validated
		http.Error(w, "cannot create upload folder", http.StatusInternalServerError)
		return
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) //nolint:gosec // rooted in the private upload directory; no traversal components
	if err != nil {
		http.Error(w, "file already uploaded", http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMusicFileSize)
	n, copyErr := io.Copy(f, r.Body)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n == 0 {
		_ = os.Remove(dest) //nolint:gosec // same validated staging path as above
		http.Error(w, "incomplete or oversized upload", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func stagedFiles(dir string) ([]string, error) {
	root := filepath.Join(dir, "files")
	var files []string
	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlinks are not allowed in uploads")
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	})
	sort.Strings(files)
	return files, err
}

func firstUploadTag(tags map[string][]string, names ...string) string {
	for _, name := range names {
		for key, values := range tags {
			if strings.EqualFold(key, name) && len(values) > 0 && strings.TrimSpace(values[0]) != "" {
				return strings.TrimSpace(values[0])
			}
		}
	}
	return ""
}

func inspectStagedTrack(dir, rel string) (uploadedTrack, error) {
	tags, err := taglib.ReadTags(filepath.Join(dir, "files", filepath.FromSlash(rel)))
	if err != nil {
		return uploadedTrack{}, err
	}
	track := uploadedTrack{Path: rel,
		Title: firstUploadTag(tags, "TITLE"), Artist: firstUploadTag(tags, "ARTIST"),
		Album: firstUploadTag(tags, "ALBUM"), AlbumArtist: firstUploadTag(tags, "ALBUMARTIST", "ALBUM ARTIST")}
	for key, values := range tags {
		if strings.EqualFold(key, "RELEASETYPE") {
			track.IsAudiobook = slices.ContainsFunc(values, func(value string) bool { return strings.EqualFold(value, "audiobook") })
		}
	}
	track.NeedsMatch = track.Title == "" || track.Artist == "" || track.Album == ""
	return track, nil
}

var discFolder = regexp.MustCompile(`(?i)^(cd|disc|disk)[ _.-]?\d+$`)

func uploadDiscFolder(rel string) string {
	disc := path.Base(path.Dir(rel))
	if discFolder.MatchString(disc) {
		return disc
	}
	return ""
}

func albumSourceGroup(rel, album string) string {
	dir := path.Dir(rel)
	if discFolder.MatchString(path.Base(dir)) {
		dir = path.Dir(dir)
	}
	return dir + "\x00" + album
}

func groupCompilations(tracks []uploadedTrack) {
	groups := map[string]map[string]bool{}
	for _, track := range tracks {
		if track.Album == "" || track.AlbumArtist != "" || track.Artist == "" {
			continue
		}
		key := albumSourceGroup(track.Path, track.Album)
		if groups[key] == nil {
			groups[key] = map[string]bool{}
		}
		groups[key][track.Artist] = true
	}
	for i := range tracks {
		if len(groups[albumSourceGroup(tracks[i].Path, tracks[i].Album)]) > 1 && tracks[i].AlbumArtist == "" {
			tracks[i].AlbumArtist = "Various Artists"
		}
	}
}

func (api *Router) previewMusicUpload(w http.ResponseWriter, r *http.Request) {
	_, dir, err := readUploadSession(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	files, err := stagedFiles(dir)
	if err != nil {
		http.Error(w, "cannot read uploaded files", http.StatusBadRequest)
		return
	}
	tracks := make([]uploadedTrack, 0)
	for _, rel := range files {
		if !musicExtensions[strings.ToLower(path.Ext(rel))] {
			continue
		}
		track, err := inspectStagedTrack(dir, rel)
		if err != nil {
			http.Error(w, "cannot read metadata in "+rel, http.StatusUnprocessableEntity)
			return
		}
		tracks = append(tracks, track)
	}
	// An album with several track artists and no Album Artist is a compilation.
	groupCompilations(tracks)
	writeUploadJSON(w, map[string]any{"tracks": tracks, "files": files})
}

func safeMusicComponent(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(value)
	value = strings.Trim(value, ". ")
	if value == "" || len(value) > 180 || strings.ContainsRune(value, '\x00') {
		return "", errors.New("artist, album and filenames must be safe path components")
	}
	return value, nil
}

// Reject pre-existing symlinked directories, including the library root. A
// library upload may never escape its configured local root through a symlink.
func ensureUploadDirectory(root string, parts ...string) (string, error) {
	current := root
	for _, part := range append([]string{""}, parts...) {
		if part != "" {
			current = filepath.Join(current, part)
		}
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0755); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("library folder is not a regular directory")
		}
	}
	return current, nil
}

// publishUploadFile uses a hard link when available so a complete temporary
// file appears atomically. Filesystems such as ExFAT do not support links, so
// fall back to an exclusive copy without replacing a concurrent upload.
func publishUploadFile(tmp, dest string) (alreadyExists bool, err error) {
	return publishUploadFileWithLink(tmp, dest, os.Link)
}

func publishUploadFileWithLink(tmp, dest string, link func(string, string) error) (alreadyExists bool, err error) {
	err = link(tmp, dest)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrExist) {
		return true, nil
	}

	in, err := os.Open(tmp)
	if err != nil {
		return false, err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return false, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return false, closeErr
	}
	return false, nil
}

type importRequest struct {
	Tracks []uploadedTrack `json:"tracks"`
}

func validateUploadAssignments(files []string, tracks []uploadedTrack) (map[string]uploadedTrack, error) {
	staged := make(map[string]bool, len(files))
	for _, name := range files {
		staged[name] = true
	}
	assignments := map[string]uploadedTrack{}
	for _, track := range tracks {
		if !staged[track.Path] || !musicExtensions[strings.ToLower(path.Ext(track.Path))] || assignments[track.Path].Path != "" {
			return nil, errors.New("invalid track selection")
		}
		for _, component := range []string{track.Artist, track.Album} {
			if _, err := safeMusicComponent(component); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(track.Title) == "" {
			return nil, errors.New("track title is required")
		}
		if track.AlbumArtist != "" {
			if _, err := safeMusicComponent(track.AlbumArtist); err != nil {
				return nil, err
			}
		}
		assignments[track.Path] = track
	}
	for _, name := range files {
		if musicExtensions[strings.ToLower(path.Ext(name))] && assignments[name].Path == "" {
			return nil, errors.New("every audio file needs metadata")
		}
	}
	return assignments, nil
}

func assignUploadCompanions(files []string, assignments map[string]uploadedTrack) map[string]uploadedTrack {
	companions := map[string]uploadedTrack{}
	for _, name := range files {
		if musicExtensions[strings.ToLower(path.Ext(name))] {
			continue
		}
		ambiguous := false
		for source, track := range assignments {
			if path.Dir(name) != path.Dir(source) && !strings.HasPrefix(path.Dir(source), path.Dir(name)+"/") {
				continue
			}
			if prev, ok := companions[name]; ok && (prev.Album != track.Album || prev.AlbumArtist != track.AlbumArtist) {
				ambiguous = true
				break
			}
			companions[name] = track
		}
		if ambiguous {
			delete(companions, name)
		}
	}
	return companions
}

// Only missing metadata is filled, except for the explicit audiobook category.
// Changes are made to the staged copy before it becomes visible to the scanner.
func writeStagedTrackTags(src string, track uploadedTrack, albumArtist string) error {
	existing, err := taglib.ReadTags(src)
	if err != nil {
		return err
	}
	changes := map[string][]string{}
	for key, value := range map[string]string{"TITLE": track.Title, "ARTIST": track.Artist, "ALBUM": track.Album, "ALBUMARTIST": albumArtist} {
		if firstUploadTag(existing, key) == "" {
			changes[key] = []string{value}
		}
	}
	var releaseTypes []string
	for key, values := range existing {
		if strings.EqualFold(key, "RELEASETYPE") {
			releaseTypes = values
			break
		}
	}
	hasAudiobook := slices.ContainsFunc(releaseTypes, func(value string) bool { return strings.EqualFold(value, "audiobook") })
	if track.IsAudiobook && !hasAudiobook {
		changes["RELEASETYPE"] = append(slices.Clone(releaseTypes), "audiobook")
	} else if !track.IsAudiobook && hasAudiobook {
		changes["RELEASETYPE"] = slices.DeleteFunc(slices.Clone(releaseTypes), func(value string) bool { return strings.EqualFold(value, "audiobook") })
	}
	if len(changes) == 0 {
		return nil
	}
	return taglib.WriteTags(src, changes, 0)
}

func (api *Router) importMusicUpload(w http.ResponseWriter, r *http.Request) {
	session, dir, err := readUploadSession(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lib, err := api.ds.Library().Get(r.Context(), session.LibraryID)
	if err != nil {
		http.Error(w, "library not found", http.StatusNotFound)
		return
	}
	var input importRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&input); err != nil {
		http.Error(w, "invalid import", http.StatusBadRequest)
		return
	}
	// Fingerprint matches may have filled artists after the initial preview.
	groupCompilations(input.Tracks)
	files, err := stagedFiles(dir)
	if err != nil {
		http.Error(w, "cannot read upload", http.StatusBadRequest)
		return
	}
	assignments, err := validateUploadAssignments(files, input.Tracks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Ambiguous companion folders (multiple albums) are reported, not guessed.
	companions := assignUploadCompanions(files, assignments)
	imported, skipped, unmatched := []string{}, []string{}, []string{}
	for _, name := range files {
		track, ok := assignments[name]
		if !ok {
			track, ok = companions[name]
		}
		if !ok {
			unmatched = append(unmatched, name)
			continue
		}
		artist := track.AlbumArtist
		if artist == "" {
			artist = track.Artist
		}
		if artist == "" {
			artist = "Various Artists"
		}
		artist, _ = safeMusicComponent(artist)
		albumName, _ := safeMusicComponent(track.Album)
		dirs := []string{artist, albumName}
		if disc := uploadDiscFolder(name); disc != "" {
			dirs = append(dirs, disc)
		}
		folder, err := ensureUploadDirectory(lib.Path, dirs...)
		if err != nil {
			http.Error(w, "library is not writable: "+err.Error(), http.StatusConflict)
			return
		}
		filename := path.Base(name)
		if _, err = safeMusicComponent(filename); err != nil {
			unmatched = append(unmatched, name)
			continue
		}
		dest := filepath.Join(folder, filename)
		if info, err := os.Lstat(dest); err == nil || info != nil {
			skipped = append(skipped, name)
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			http.Error(w, "could not check destination", http.StatusInternalServerError)
			return
		}
		src := filepath.Join(dir, "files", filepath.FromSlash(name))
		// Write approved missing tags only to the staged copy. Existing tags remain
		// untouched, and failures leave the original staged file available to retry.
		if musicExtensions[strings.ToLower(path.Ext(name))] {
			if err := writeStagedTrackTags(src, track, artist); err != nil {
				http.Error(w, "could not write track metadata", http.StatusUnprocessableEntity)
				return
			}
		}
		// Copy to a hidden sibling before publishing the complete file. Link is
		// atomic and fails if another upload created the name in the meantime.
		out, err := os.CreateTemp(folder, ".music-upload-*")
		if err != nil {
			http.Error(w, "cannot create library file", http.StatusConflict)
			return
		}
		tmp := out.Name()
		in, err := os.Open(src)
		if err == nil {
			_, err = io.Copy(out, in)
			_ = in.Close()
		}
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(tmp)
			http.Error(w, "could not store track", http.StatusInternalServerError)
			return
		}
		alreadyExists, err := publishUploadFile(tmp, dest)
		_ = os.Remove(tmp)
		if alreadyExists {
			skipped = append(skipped, name)
			continue
		}
		if err != nil {
			log.Error(r.Context(), "Could not publish uploaded track", "track", name, err)
			http.Error(w, "could not publish track", http.StatusInternalServerError)
			return
		}
		imported = append(imported, name)
	}
	if len(unmatched) == 0 {
		_ = os.RemoveAll(dir)
	}
	writeUploadJSON(w, map[string]any{"imported": imported, "skipped": skipped, "unmatched": unmatched, "libraryId": lib.ID})
}

func (api *Router) cancelMusicUpload(w http.ResponseWriter, r *http.Request) {
	_, dir, err := readUploadSession(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		http.Error(w, "cannot cancel upload", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type fingerprintResult struct {
	Fingerprint string `json:"fingerprint"`
	Duration    int    `json:"duration"`
}

type matchRequest struct {
	Path string `json:"path"`
}
type matchCandidate struct {
	RecordingID string  `json:"-"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	Score       float64 `json:"score"`
}

var singleLine = regexp.MustCompile(`\s+`)

func (api *Router) matchMusicTrack(w http.ResponseWriter, r *http.Request) {
	if conf.Server.AcoustIDKey == "" {
		http.Error(w, "AcoustID key is not configured", http.StatusServiceUnavailable)
		return
	}
	if _, err := exec.LookPath("fpcalc"); err != nil {
		http.Error(w, "fpcalc is not installed", http.StatusServiceUnavailable)
		return
	}
	_, dir, err := readUploadSession(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var input matchRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&input) != nil {
		http.Error(w, "invalid track", http.StatusBadRequest)
		return
	}
	rel, err := validUploadPath(input.Path)
	if err != nil || !musicExtensions[strings.ToLower(path.Ext(rel))] {
		http.Error(w, "invalid track", http.StatusBadRequest)
		return
	}
	file := filepath.Join(dir, "files", filepath.FromSlash(rel))
	if _, err := os.Stat(file); err != nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "fpcalc", "-json", file).Output() //nolint:gosec // validated staged path
	if err != nil {
		http.Error(w, "could not fingerprint track", http.StatusUnprocessableEntity)
		return
	}
	var fp fingerprintResult
	if json.Unmarshal(output, &fp) != nil || fp.Duration < 1 || fp.Fingerprint == "" {
		http.Error(w, "invalid fingerprint", http.StatusUnprocessableEntity)
		return
	}
	values := url.Values{"client": {conf.Server.AcoustIDKey}, "duration": {strconv.Itoa(fp.Duration)}, "fingerprint": {fp.Fingerprint}, "meta": {"recordings+releasegroups"}}
	if err := acoustIDLimiter.Wait(ctx); err != nil {
		http.Error(w, "fingerprint lookup cancelled", http.StatusRequestTimeout)
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.acoustid.org/v2/lookup", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, "fingerprint lookup unavailable", http.StatusBadGateway)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()
	var result struct {
		Status  string `json:"status"`
		Results []struct {
			Score      float64 `json:"score"`
			Recordings []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Duration int    `json:"duration"`
				Artists  []struct {
					Name string `json:"name"`
				} `json:"artists"`
				ReleaseGroups []struct {
					Title string `json:"title"`
				} `json:"releasegroups"`
			} `json:"recordings"`
		} `json:"results"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil || result.Status != "ok" {
		http.Error(w, "invalid fingerprint response", http.StatusBadGateway)
		return
	}
	candidates := make([]matchCandidate, 0)
	for _, found := range result.Results {
		for _, recording := range found.Recordings {
			if len(recording.Artists) == 0 || len(recording.ReleaseGroups) == 0 || recording.Title == "" || (recording.Duration > 0 && (recording.Duration > fp.Duration+3 || recording.Duration < fp.Duration-3)) {
				continue
			}
			candidates = append(candidates, matchCandidate{RecordingID: recording.ID, Title: singleLine.ReplaceAllString(recording.Title, " "), Artist: recording.Artists[0].Name, Album: recording.ReleaseGroups[0].Title, Score: found.Score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > 0 {
		enrichMatchFromMusicBrainz(ctx, &candidates[0])
	}
	writeUploadJSON(w, map[string]any{"candidates": candidates})
}

func enrichMatchFromMusicBrainz(ctx context.Context, candidate *matchCandidate) {
	if candidate.RecordingID == "" || musicBrainzLimiter.Wait(ctx) != nil {
		return
	}
	lookup, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://musicbrainz.org/ws/2/recording/"+url.PathEscape(candidate.RecordingID)+"?inc=artist-credits+releases&fmt=json", nil)
	if err != nil {
		return
	}
	lookup.Header.Set("User-Agent", "Music-Server/0.1 (https://github.com/navidrome/navidrome)")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(lookup)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return
	}
	var mb struct {
		Title        string `json:"title"`
		ArtistCredit []struct {
			Name string `json:"name"`
		} `json:"artist-credit"`
		Releases []struct {
			Title string `json:"title"`
		} `json:"releases"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&mb) != nil {
		return
	}
	if mb.Title != "" {
		candidate.Title = mb.Title
	}
	if len(mb.ArtistCredit) > 0 && mb.ArtistCredit[0].Name != "" {
		candidate.Artist = mb.ArtistCredit[0].Name
	}
	if len(mb.Releases) > 0 && mb.Releases[0].Title != "" {
		candidate.Album = mb.Releases[0].Title
	}
}
