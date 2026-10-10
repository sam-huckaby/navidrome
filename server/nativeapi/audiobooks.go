package nativeapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

func (api *Router) addAudiobookCategoryRoute(r chi.Router) {
	r.Put("/album/{id}/audiobook", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			IsAudiobook *bool `json:"isAudiobook"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || input.IsAudiobook == nil {
			http.Error(w, "isAudiobook must be a boolean", http.StatusBadRequest)
			return
		}
		id := chi.URLParam(r, "id")
		err := api.ds.WithTx(func(tx model.DataStore) error {
			if _, err := tx.Album().Get(r.Context(), id); err != nil {
				return err
			}
			return tx.Album().SetAudiobookOverride(r.Context(), id, *input.IsAudiobook)
		}, "set audiobook category")
		if errors.Is(err, model.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "could not update audiobook", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (api *Router) addAudiobookProgressRoute(r chi.Router) {
	r.Route("/audiobook/{id}/progress", func(r chi.Router) {
		r.Get("/", api.getAudiobookProgress)
		r.Put("/", api.putAudiobookProgress)
	})
}

func (api *Router) getAudiobookProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	album, err := api.ds.Album().Get(r.Context(), id)
	if err != nil || !album.IsAudiobook {
		http.NotFound(w, r)
		return
	}
	user, _ := request.UserFrom(r.Context())
	progress, err := api.ds.Album().GetAudiobookProgress(r.Context(), id, user.ID)
	if err != nil {
		http.Error(w, "could not load audiobook progress", http.StatusInternalServerError)
		return
	}
	if progress == nil {
		progress = &model.AudiobookProgress{AlbumID: id}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(progress)
}

func (api *Router) putAudiobookProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	album, err := api.ds.Album().Get(r.Context(), id)
	if err != nil || !album.IsAudiobook {
		http.NotFound(w, r)
		return
	}
	var input struct {
		TrackID    string `json:"trackId"`
		PositionMs int64  `json:"positionMs"`
		Completed  bool   `json:"completed"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || input.TrackID == "" || input.PositionMs < 0 {
		http.Error(w, "invalid audiobook position", http.StatusBadRequest)
		return
	}
	track, err := api.ds.MediaFile().Get(r.Context(), input.TrackID)
	if err != nil || track.AlbumID != id {
		http.Error(w, "chapter not found in audiobook", http.StatusBadRequest)
		return
	}
	if track.Duration > 0 && input.PositionMs > int64(track.Duration*1000)+5000 {
		http.Error(w, "position exceeds chapter duration", http.StatusBadRequest)
		return
	}
	user, _ := request.UserFrom(r.Context())
	progress := model.AudiobookProgress{AlbumID: id, UserID: user.ID,
		TrackID: input.TrackID, PositionMs: input.PositionMs, Completed: input.Completed, UpdatedAt: time.Now()}
	if err := api.ds.Album().SaveAudiobookProgress(r.Context(), progress); err != nil {
		http.Error(w, "could not save audiobook progress", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
