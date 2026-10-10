package tests

import (
	"context"

	"github.com/navidrome/navidrome/model"
)

// MockTagRepo records the QueryOptions passed to GetAll, mirroring MockArtistRepo, so tests can
// assert on which filters a caller attached (e.g. a library scope).
type MockTagRepo struct {
	model.TagRepository
	Data    model.TagList
	Options model.QueryOptions
	Err     error
}

func (r *MockTagRepo) GetAll(_ context.Context, _ model.TagName, options ...model.QueryOptions) (model.TagList, error) {
	if len(options) > 0 {
		r.Options = options[0]
	}
	if r.Err != nil {
		return nil, r.Err
	}
	return r.Data, nil
}

func (r *MockTagRepo) Add(_ context.Context, _ int, tags ...model.Tag) error {
	r.Data = append(r.Data, tags...)
	return r.Err
}

func (r *MockTagRepo) UpdateCounts(context.Context) error { return r.Err }
