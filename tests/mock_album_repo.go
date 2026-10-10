package tests

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
)

func CreateMockAlbumRepo() *MockAlbumRepo {
	return &MockAlbumRepo{
		Data: make(map[string]*model.Album),
	}
}

type MockAlbumRepo struct {
	model.AlbumRepository
	Data                    map[string]*model.Album
	TagOverrides            map[string]model.Tags
	AudiobookOverrides      map[string]bool
	AudiobookProgress       map[string]model.AudiobookProgress
	All                     model.Albums
	Err                     bool
	Options                 model.QueryOptions
	optionsMu               sync.Mutex
	SearchQuery             string            // last query passed to Search
	ReassignAnnotationCalls map[string]string // prevID -> newID
	CopyAttributesCalls     map[string]string // fromID -> toID
}

func (m *MockAlbumRepo) SetAudiobookOverride(_ context.Context, id string, audiobook bool) error {
	if m.AudiobookOverrides == nil {
		m.AudiobookOverrides = map[string]bool{}
	}
	m.AudiobookOverrides[id] = audiobook
	if album := m.Data[id]; album != nil {
		album.IsAudiobook = audiobook
	}
	return nil
}

func (m *MockAlbumRepo) ApplyAudiobookOverride(_ context.Context, album *model.Album) error {
	album.IsAudiobook = slices.ContainsFunc(album.Tags.Values(model.TagReleaseType),
		func(value string) bool { return strings.EqualFold(value, "audiobook") })
	if kind, found := m.AudiobookOverrides[album.ID]; found {
		album.IsAudiobook = kind
	}
	return nil
}

func (m *MockAlbumRepo) GetAudiobookProgress(_ context.Context, albumID, userID string) (*model.AudiobookProgress, error) {
	progress, found := m.AudiobookProgress[albumID+":"+userID]
	if !found {
		return nil, nil
	}
	return &progress, nil
}

func (m *MockAlbumRepo) SaveAudiobookProgress(_ context.Context, progress model.AudiobookProgress) error {
	if m.AudiobookProgress == nil {
		m.AudiobookProgress = map[string]model.AudiobookProgress{}
	}
	m.AudiobookProgress[progress.AlbumID+":"+progress.UserID] = progress
	return nil
}

func (m *MockAlbumRepo) GetTagOverrides(_ context.Context, id string) (model.Tags, error) {
	return m.TagOverrides[id], nil
}

func (m *MockAlbumRepo) SetTagOverrides(_ context.Context, id string, tags model.Tags) error {
	if m.TagOverrides == nil {
		m.TagOverrides = make(map[string]model.Tags)
	}
	m.TagOverrides[id] = tags
	return nil
}

func (m *MockAlbumRepo) ApplyTagOverrides(_ context.Context, album *model.Album) error {
	for name, values := range m.TagOverrides[album.ID] {
		if len(values) == 0 {
			delete(album.Tags, name)
		} else {
			if album.Tags == nil {
				album.Tags = model.Tags{}
			}
			album.Tags[name] = values
		}
	}
	album.Genre, album.Genres = album.Tags.ToGenres()
	return nil
}

func (m *MockAlbumRepo) SetError(err bool) {
	m.Err = err
}

func (m *MockAlbumRepo) SetData(albums model.Albums) {
	m.Data = make(map[string]*model.Album, len(albums))
	m.All = albums
	for i, a := range m.All {
		m.Data[a.ID] = &m.All[i]
	}
}

func (m *MockAlbumRepo) Exists(_ context.Context, id string) (bool, error) {
	if m.Err {
		return false, errors.New("unexpected error")
	}
	_, found := m.Data[id]
	return found, nil
}

func (m *MockAlbumRepo) Get(_ context.Context, id string) (*model.Album, error) {
	if m.Err {
		return nil, errors.New("unexpected error")
	}
	if d, ok := m.Data[id]; ok {
		return d, nil
	}
	return nil, model.ErrNotFound
}

func (m *MockAlbumRepo) Put(_ context.Context, al *model.Album) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	if al.ID == "" {
		al.ID = id.NewRandom()
	}
	if m.Data == nil {
		m.Data = make(map[string]*model.Album)
	}
	m.Data[al.ID] = al
	return nil
}

func (m *MockAlbumRepo) GetAll(_ context.Context, qo ...model.QueryOptions) (model.Albums, error) {
	if len(qo) > 0 {
		// Recording the last options is a read-path write, and callers resolve concurrently.
		m.optionsMu.Lock()
		m.Options = qo[0]
		m.optionsMu.Unlock()
	}
	if m.Err {
		return nil, errors.New("unexpected error")
	}
	return m.All, nil
}

func (m *MockAlbumRepo) GetCursor(ctx context.Context, qo ...model.QueryOptions) (model.AlbumCursor, error) {
	res, err := m.GetAll(ctx, qo...)
	if err != nil {
		return nil, err
	}
	return func(yield func(model.Album, error) bool) {
		for _, a := range res {
			if !yield(a, nil) {
				return
			}
		}
	}, nil
}

func (m *MockAlbumRepo) IncPlayCount(_ context.Context, id string, timestamp time.Time) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	if d, ok := m.Data[id]; ok {
		d.PlayCount++
		d.PlayDate = &timestamp
		return nil
	}
	return model.ErrNotFound
}
func (m *MockAlbumRepo) CountAll(_ context.Context, _ ...model.QueryOptions) (int64, error) {
	return int64(len(m.All)), nil
}

func (m *MockAlbumRepo) GetTouchedAlbums(_ context.Context, libID int) (model.AlbumCursor, error) {
	if m.Err {
		return nil, errors.New("unexpected error")
	}
	return func(yield func(model.Album, error) bool) {
		for _, a := range m.Data {
			if a.ID == "error" {
				if !yield(*a, errors.New("error")) {
					break
				}
				continue
			}
			if a.LibraryID != libID {
				continue
			}
			if !yield(*a, nil) {
				break
			}
		}
	}, nil
}

func (m *MockAlbumRepo) UpdateExternalInfo(ctx context.Context, album *model.Album) error {
	return m.Put(ctx, album)
}

func (m *MockAlbumRepo) Search(_ context.Context, q string, options ...model.QueryOptions) (model.Albums, error) {
	m.SearchQuery = q
	if len(options) > 0 {
		m.Options = options[0]
	}
	if m.Err {
		return nil, errors.New("unexpected error")
	}
	// Simple mock implementation - just return all albums for testing
	return m.All, nil
}

// ReassignAnnotation reassigns annotations from one album to another
func (m *MockAlbumRepo) ReassignAnnotation(_ context.Context, prevID string, newID string) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	// Mock implementation - track the reassignment calls
	if m.ReassignAnnotationCalls == nil {
		m.ReassignAnnotationCalls = make(map[string]string)
	}
	m.ReassignAnnotationCalls[prevID] = newID
	return nil
}

// CopyAttributes copies attributes from one album to another
func (m *MockAlbumRepo) CopyAttributes(_ context.Context, fromID, toID string, columns ...string) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	from, ok := m.Data[fromID]
	if !ok {
		return model.ErrNotFound
	}
	to, ok := m.Data[toID]
	if !ok {
		return model.ErrNotFound
	}
	for _, col := range columns {
		switch col {
		case "created_at":
			to.CreatedAt = from.CreatedAt
		}
	}
	if m.CopyAttributesCalls == nil {
		m.CopyAttributesCalls = make(map[string]string)
	}
	m.CopyAttributesCalls[fromID] = toID
	return nil
}

// SetRating sets the rating for an album
func (m *MockAlbumRepo) SetRating(_ context.Context, rating int, itemID string) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	if d, ok := m.Data[itemID]; ok {
		d.Rating = rating
	}
	return nil
}

// SetStar sets the starred status for albums
func (m *MockAlbumRepo) SetStar(_ context.Context, starred bool, itemIDs ...string) error {
	if m.Err {
		return errors.New("unexpected error")
	}
	for _, id := range itemIDs {
		if d, ok := m.Data[id]; ok {
			d.Starred = starred
		}
	}
	return nil
}

func (m *MockAlbumRepo) GetYears(_ context.Context, libraryIDs ...int) ([]int, error) {
	if m.Err {
		return nil, errors.New("error")
	}
	return []int{}, nil
}

var _ model.AlbumRepository = (*MockAlbumRepo)(nil)
