package nativeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/squirrel"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
)

var editableAlbumTags = map[model.TagName]bool{
	model.TagGenre: true, model.TagMood: true, model.TagGrouping: true,
	model.TagReleaseType: true, model.TagRecordLabel: true,
	model.TagMedia: true, model.TagAlbumVersion: true,
}

func (api *Router) addAlbumTagRoutes(r chi.Router) {
	r.Route("/album/{id}/tags", func(r chi.Router) {
		r.Get("/", api.getAlbumTags)
		r.Put("/", api.putAlbumTags)
		r.Delete("/", api.deleteAlbumTags)
	})
}

func (api *Router) getAlbumTags(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	album, err := api.ds.Album().Get(r.Context(), id)
	if errors.Is(err, model.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "could not load album", http.StatusInternalServerError)
		return
	}
	overrides, err := api.ds.Album().GetTagOverrides(r.Context(), id)
	if err != nil {
		http.Error(w, "could not load album tag overrides", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Tags      model.Tags `json:"tags"`
		Overrides model.Tags `json:"overrides"`
	}{album.Tags, overrides})
}

// A JSON null removes an override, while [] explicitly clears the tag on the album.
func parseAlbumTagChanges(w http.ResponseWriter, r *http.Request) (map[model.TagName]*[]string, error) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return nil, errors.New("expected a JSON object of tag changes")
	}
	if len(raw) == 0 {
		return nil, errors.New("no tags specified")
	}
	changes := make(map[model.TagName]*[]string, len(raw))
	for key, value := range raw {
		name := model.TagName(key)
		_, isAlbumTag := model.AlbumLevelTags()[name]
		if !editableAlbumTags[name] || !isAlbumTag {
			return nil, fmt.Errorf("tag %q is not editable", key)
		}
		if string(value) == "null" {
			changes[name] = nil
			continue
		}
		var values []string
		if err := json.Unmarshal(value, &values); err != nil || values == nil || len(values) > 32 {
			return nil, fmt.Errorf("invalid values for %q", key)
		}
		clean := make([]string, 0, len(values))
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" || utf8.RuneCountInString(v) > 256 {
				return nil, fmt.Errorf("invalid value for %q", key)
			}
			if !slices.Contains(clean, v) {
				clean = append(clean, v)
			}
		}
		changes[name] = &clean
	}
	return changes, nil
}

func (api *Router) putAlbumTags(w http.ResponseWriter, r *http.Request) {
	changes, err := parseAlbumTagChanges(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	api.updateAlbumTags(w, r, changes, false)
}

func (api *Router) deleteAlbumTags(w http.ResponseWriter, r *http.Request) {
	api.updateAlbumTags(w, r, nil, true)
}

func (api *Router) updateAlbumTags(w http.ResponseWriter, r *http.Request, changes map[model.TagName]*[]string, clear bool) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	err := api.ds.WithTx(func(tx model.DataStore) error {
		album, err := tx.Album().Get(ctx, id)
		if err != nil {
			return err
		}
		files, err := tx.MediaFile().GetAll(ctx, model.QueryOptions{Filters: squirrel.Eq{"album_id": id}})
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return errAlbumHasNoTracks
		}
		overrides, err := tx.Album().GetTagOverrides(ctx, id)
		if err != nil {
			return err
		}
		if overrides == nil {
			overrides = model.Tags{}
		}
		if clear {
			overrides = model.Tags{}
		} else {
			for name, values := range changes {
				if values == nil {
					delete(overrides, name)
				} else {
					overrides[name] = *values
				}
			}
		}
		if err := tx.Album().SetTagOverrides(ctx, id, overrides); err != nil {
			return err
		}
		// Rebuild from the imported track metadata, never from the already overridden
		// album. Removing an override then immediately restores the original value.
		album.Tags = files.ToAlbum().Tags
		if err := tx.Album().ApplyTagOverrides(ctx, album); err != nil {
			return err
		}
		if err := tx.Album().ApplyAudiobookOverride(ctx, album); err != nil {
			return err
		}
		if err := tx.Tag().Add(ctx, album.LibraryID, album.Tags.FlattenAll()...); err != nil {
			return err
		}
		if err := tx.Album().Put(ctx, album); err != nil {
			return err
		}
		return tx.Tag().UpdateCounts(ctx)
	}, "update album tags")
	if errors.Is(err, model.ErrNotFound) {
		http.NotFound(w, r)
	} else if errors.Is(err, errAlbumHasNoTracks) {
		http.Error(w, "album has no tracks", http.StatusConflict)
	} else if err != nil {
		http.Error(w, "could not update album tags", http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

var errAlbumHasNoTracks = errors.New("album has no tracks")
