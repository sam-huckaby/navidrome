package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/deluan/rest"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/slice"
	"github.com/navidrome/navidrome/utils/str"
	"github.com/pocketbase/dbx"
)

type albumRepository struct {
	sqlRepository
}

type dbAlbum struct {
	*model.Album `structs:",flatten"`
	Discs        string `structs:"-" json:"discs"`
	Participants string `structs:"-" json:"-"`
	Tags         string `structs:"-" json:"-"`
	FolderIDs    string `structs:"-" json:"-"`
	// dbx maps columns to fields by name; RGAlbumGain doesn't convert to
	// rg_album_gain, so shim fields carry the read and PostScan copies them over.
	RgAlbumGain *float64 `structs:"-" json:"-"`
	RgAlbumPeak *float64 `structs:"-" json:"-"`
}

func (a *dbAlbum) PostScan() error {
	var err error
	if a.Discs != "" {
		if err = json.Unmarshal([]byte(a.Discs), &a.Album.Discs); err != nil {
			return fmt.Errorf("parsing album discs from db: %w", err)
		}
	}
	a.Album.Participants, err = unmarshalParticipants(a.Participants)
	if err != nil {
		return fmt.Errorf("parsing album from db: %w", err)
	}
	if a.Tags != "" {
		a.Album.Tags, err = unmarshalTags(a.Tags)
		if err != nil {
			return fmt.Errorf("parsing album from db: %w", err)
		}
		a.Genre, a.Genres = a.Album.Tags.ToGenres()
	}
	if a.FolderIDs != "" {
		var ids []string
		if err = json.Unmarshal([]byte(a.FolderIDs), &ids); err != nil {
			return fmt.Errorf("parsing album folder_ids from db: %w", err)
		}
		a.Album.FolderIDs = ids
	}
	a.Album.RGAlbumGain = a.RgAlbumGain
	a.Album.RGAlbumPeak = a.RgAlbumPeak
	return nil
}

func (a *dbAlbum) PostMapArgs(args map[string]any) error {
	fullText := []string{a.Name, a.SortAlbumName, a.AlbumArtist}
	participantNames := a.Album.Participants.AllNames()
	fullText = append(fullText, participantNames...)
	fullText = append(fullText, slices.Collect(maps.Values(a.Album.Discs))...)
	fullText = append(fullText, a.Album.Tags[model.TagAlbumVersion]...)
	fullText = append(fullText, a.Album.Tags[model.TagCatalogNumber]...)
	args["full_text"] = formatFullText(fullText...)
	args["search_participants"] = strings.Join(participantNames, " ")
	args["search_normalized"] = str.NormalizeForFTS(a.Name, a.AlbumArtist)

	args["tags"] = marshalTags(a.Album.Tags)
	args["participants"] = marshalParticipants(a.Album.Participants)

	folderIDs, err := json.Marshal(a.Album.FolderIDs)
	if err != nil {
		return fmt.Errorf("marshalling album folder_ids: %w", err)
	}
	args["folder_ids"] = string(folderIDs)

	b, err := json.Marshal(a.Album.Discs)
	if err != nil {
		return fmt.Errorf("marshalling album discs: %w", err)
	}
	args["discs"] = string(b)
	return nil
}

type dbAlbums []dbAlbum

func (as dbAlbums) toModels() model.Albums {
	return slice.Map(as, func(a dbAlbum) model.Album { return *a.Album })
}

func NewAlbumRepository(db dbx.Builder) model.AlbumRepository {
	r := &albumRepository{}
	r.db = db
	r.tableName = "album"
	r.registerModel(&model.Album{}, albumFilters())
	r.setSortMappings(map[string]string{
		"name":         "order_album_name, order_album_artist_name",
		"artist":       "compilation, order_album_artist_name, order_album_name",
		"album_artist": "compilation, order_album_artist_name, order_album_name",
		// TODO Rename this to just year (or date)
		"max_year":       "coalesce(nullif(original_date,''), cast(max_year as text)), release_date, " + naturalSort("album.name"),
		"random":         "random",
		"recently_added": recentlyAddedSort(),
		"starred_at":     "starred, starred_at",
		"rated_at":       "rating, rated_at",
	})
	return r
}

var albumFilters = sync.OnceValue(func() map[string]filterFunc {
	filters := map[string]filterFunc{
		"id":              idFilter("album"),
		"name":            fullTextFilter("album", "mbz_album_id", "mbz_release_group_id"),
		"compilation":     booleanFilter,
		"artist_id":       artistFilter,
		"year":            yearFilter,
		"recently_played": recentlyPlayedFilter,
		"starred":         annotationBoolFilter("starred"),
		"has_rating":      annotationBoolFilter("rating"),
		"played":          annotationBoolFilter("play_count"),
		"missing":         booleanFilter,
		"genre_id":        genreFilter(AlbumGenres),
		"role_total_id":   allRolesFilter,
		"library_id":      libraryIdFilter,
		"audiobook":       func(_ string, value any) Sqlizer { return Eq{"album.is_audiobook": isTrueFilterValue(value)} },
	}
	// Add all album tags as filters
	for tag := range model.AlbumLevelTags() {
		filters[string(tag)] = tagIDFilter
	}

	for role := range model.AllRoles {
		filters["role_"+role+"_id"] = artistRoleFilter
	}

	return filters
})

func recentlyAddedSort() string {
	if conf.Server.RecentlyAddedByModTime {
		return "album.updated_at, album.id"
	}
	return "album.created_at, album.id"
}

func recentlyPlayedFilter(string, any) Sqlizer {
	return Gt{"play_count": 0}
}

func yearFilter(_ string, value any) Sqlizer {
	return Or{
		And{
			Gt{"min_year": 0},
			LtOrEq{"min_year": value},
			GtOrEq{"max_year": value},
		},
		Eq{"max_year": value},
	}
}

func artistFilter(_ string, value any) Sqlizer {
	return ParticipantIDFilter("album", value, model.RoleAlbumArtist, model.RoleArtist)
}

func artistRoleFilter(name string, value any) Sqlizer {
	roleName := strings.TrimSuffix(strings.TrimPrefix(name, "role_"), "_id")

	// Check if the role name is valid. If not, return an invalid filter
	role, ok := model.AllRoles[roleName]
	if !ok {
		return Gt{"": nil}
	}
	return ParticipantIDFilter("album", value, role)
}

func allRolesFilter(_ string, value any) Sqlizer {
	return ParticipantIDFilter("album", value)
}

func (r *albumRepository) CountAll(ctx context.Context, options ...model.QueryOptions) (int64, error) {
	query := r.newSelect(ctx)
	query = r.applyLibraryFilter(ctx, query)
	if filtersNeedAnnotation(r.applyFilters(query, options...)) {
		query = r.withAnnotation(ctx, query, "album.id")
	}
	return r.count(ctx, query, options...)
}

func (r *albumRepository) Exists(ctx context.Context, id string) (bool, error) {
	// The exists() helper applies no library filter, so it would report rows the caller cannot see.
	c, err := r.count(ctx, r.applyLibraryFilter(ctx, r.newSelect(ctx).Where(Eq{"album.id": id})))
	return c > 0, err
}

func (r *albumRepository) Put(ctx context.Context, al *model.Album) error {
	al.ImportedAt = time.Now()
	id, err := r.put(ctx, al.ID, &dbAlbum{Album: al})
	if err != nil {
		return err
	}
	al.ID = id
	if err := r.updateParticipants(ctx, al.ID, al.Participants); err != nil {
		return err
	}
	return r.updateTags(ctx, al.ID, al.Tags)
}

func (r *albumRepository) SetAudiobookOverride(ctx context.Context, id string, audiobook bool) error {
	value := 0
	if audiobook {
		value = 1
	}
	_, err := r.db.NewQuery(`INSERT INTO album_audiobook_override (album_id, is_audiobook)
		VALUES ({:id},{:value}) ON CONFLICT(album_id) DO UPDATE SET is_audiobook=excluded.is_audiobook`).
		Bind(dbx.Params{"id": id, "value": value}).Execute()
	if err != nil {
		return err
	}
	_, err = r.db.NewQuery(`UPDATE album SET is_audiobook={:value} WHERE id={:id}`).
		Bind(dbx.Params{"id": id, "value": value}).Execute()
	return err
}

func (r *albumRepository) ApplyAudiobookOverride(ctx context.Context, album *model.Album) error {
	album.IsAudiobook = slices.ContainsFunc(album.Tags.Values(model.TagReleaseType),
		func(value string) bool { return strings.EqualFold(value, "audiobook") })
	var override struct {
		Value int `db:"is_audiobook"`
	}
	err := r.db.NewQuery(`SELECT is_audiobook FROM album_audiobook_override WHERE album_id={:id}`).
		Bind(dbx.Params{"id": album.ID}).One(&override)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	album.IsAudiobook = override.Value != 0
	return nil
}

func (r *albumRepository) GetAudiobookProgress(ctx context.Context, albumID, userID string) (*model.AudiobookProgress, error) {
	var row struct {
		TrackID    string    `db:"track_id"`
		PositionMs int64     `db:"position_ms"`
		Completed  bool      `db:"completed"`
		UpdatedAt  time.Time `db:"updated_at"`
	}
	err := r.db.NewQuery(`SELECT track_id, position_ms, completed, updated_at FROM audiobook_progress
		WHERE album_id={:album} AND user_id={:user}`).
		Bind(dbx.Params{"album": albumID, "user": userID}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.AudiobookProgress{AlbumID: albumID, UserID: userID, TrackID: row.TrackID,
		PositionMs: row.PositionMs, Completed: row.Completed, UpdatedAt: row.UpdatedAt}, nil
}

func (r *albumRepository) SaveAudiobookProgress(ctx context.Context, progress model.AudiobookProgress) error {
	_, err := r.db.NewQuery(`INSERT INTO audiobook_progress (album_id, user_id, track_id, position_ms, completed, updated_at)
		VALUES ({:album}, {:user}, {:track}, {:position}, {:completed}, {:now})
		ON CONFLICT(album_id, user_id) DO UPDATE SET track_id=excluded.track_id,
		position_ms=excluded.position_ms, completed=excluded.completed, updated_at=excluded.updated_at`).
		Bind(dbx.Params{"album": progress.AlbumID, "user": progress.UserID, "track": progress.TrackID,
			"position": progress.PositionMs, "completed": progress.Completed, "now": time.Now()}).Execute()
	return err
}

// GetTagOverrides returns only fields explicitly edited in Music: Server. An
// empty slice is an intentional override that clears the file-derived value.
func (r *albumRepository) GetTagOverrides(ctx context.Context, id string) (model.Tags, error) {
	var row struct {
		Tags string `db:"tags"`
	}
	err := r.db.NewQuery("SELECT tags FROM album_tag_override WHERE album_id={:id}").
		Bind(dbx.Params{"id": id}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Tags{}, nil
	}
	if err != nil {
		return nil, err
	}
	var tags model.Tags
	if err := json.Unmarshal([]byte(row.Tags), &tags); err != nil {
		return nil, fmt.Errorf("decoding album tag overrides: %w", err)
	}
	return tags, nil
}

func (r *albumRepository) SetTagOverrides(ctx context.Context, id string, tags model.Tags) error {
	if len(tags) == 0 {
		_, err := r.db.NewQuery("DELETE FROM album_tag_override WHERE album_id={:id}").
			Bind(dbx.Params{"id": id}).Execute()
		return err
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = r.db.NewQuery(`INSERT INTO album_tag_override (album_id,tags) VALUES ({:id},{:tags})
		ON CONFLICT(album_id) DO UPDATE SET tags=excluded.tags`).
		Bind(dbx.Params{"id": id, "tags": string(b)}).Execute()
	return err
}

func (r *albumRepository) ApplyTagOverrides(ctx context.Context, album *model.Album) error {
	overrides, err := r.GetTagOverrides(ctx, album.ID)
	if err != nil {
		return err
	}
	if len(overrides) == 0 {
		return nil
	}
	tags := make(model.Tags, len(album.Tags)+len(overrides))
	for name, values := range album.Tags {
		tags[name] = slices.Clone(values)
	}
	for name, values := range overrides {
		if len(values) == 0 {
			delete(tags, name)
		} else {
			tags[name] = slices.Clone(values)
		}
	}
	album.Tags = tags
	album.Genre, album.Genres = tags.ToGenres()
	return nil
}

// TODO Move external metadata to a separated table
func (r *albumRepository) UpdateExternalInfo(ctx context.Context, al *model.Album) error {
	_, err := r.put(ctx, al.ID, &dbAlbum{Album: al}, "description", "small_image_url", "medium_image_url", "large_image_url", "external_url", "external_info_updated_at")
	return err
}

func (r *albumRepository) selectAlbum(ctx context.Context, options ...model.QueryOptions) SelectBuilder {
	sql := r.newSelect(ctx, options...).Columns("album.*", "library.path as library_path", "library.name as library_name").
		LeftJoin("library on album.library_id = library.id")
	sql = r.withAnnotation(ctx, sql, "album.id")
	return r.applyLibraryFilter(ctx, sql)
}

func (r *albumRepository) Get(ctx context.Context, id string) (*model.Album, error) {
	res, err := r.GetAll(ctx, model.QueryOptions{Filters: Eq{"album.id": id}})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, model.ErrNotFound
	}
	return &res[0], nil
}

func (r *albumRepository) GetAll(ctx context.Context, options ...model.QueryOptions) (model.Albums, error) {
	sq := r.selectAlbum(ctx, options...)
	var res dbAlbums
	err := r.queryAll(ctx, sq, &res)
	if err != nil {
		return nil, err
	}
	albums := res.toModels()
	r.hydrateArtwork(ctx, albums)
	return albums, nil
}

func (r *albumRepository) hydrateArtwork(ctx context.Context, albums model.Albums) {
	hydrateItems(ctx, r.db, model.KindAlbumArtwork, albums,
		func(a *model.Album) (string, *model.ItemImage) { return a.ID, &a.ItemImage })
}

// getAllIDs returns the IDs of GetAll's row set, skipping its column projection and JSON decoding.
func (r *albumRepository) getAllIDs(ctx context.Context, options ...model.QueryOptions) ([]string, error) {
	sq := r.applyLibraryFilter(ctx, r.newSelect(ctx, options...).Columns("album.id"))
	if filtersNeedAnnotation(sq) {
		sq = r.withAnnotation(ctx, sq, "album.id")
	}
	ids := []string{}
	err := r.queryAllSlice(ctx, sq, &ids)
	return ids, err
}

// soleAlbumArtistFilter matches albums with exactly one album artist. The artist artwork
// resolver and the scanner's image-change enqueue must select the same albums.
var soleAlbumArtistFilter = Eq{"json_array_length(participants, '$.albumartist')": 1}

// SoleAlbumArtistFilter matches the albums where the given artist is the only album artist.
// Matches by album-artist participation, not the deprecated album_artist_id column.
func SoleAlbumArtistFilter(artistID string) Sqlizer {
	return And{ParticipantIDFilter("album", artistID, model.RoleAlbumArtist), soleAlbumArtistFilter}
}

// GetSoleAlbumArtistIDsInSubtrees matches albums by their own folder_ids, which is the resolver's
// notion of an album's folders.
func (r *albumRepository) GetSoleAlbumArtistIDsInSubtrees(ctx context.Context, lib model.Library, paths ...string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	ids := []string{}
	// Repeated IDs across chunks are fine: the queue upserts by PK.
	for chunk := range slices.Chunk(paths, subtreePathChunkSize) {
		inSubtree := Exists("json_each(album.folder_ids) je join folder on folder.id = je.value",
			folderSubtreeFilter(lib, chunk))
		// Sole album artist, so participants[0] is the only one.
		sq := Select("distinct json_extract(participants, '$.albumartist[0].id')").From("album").
			Where(And{soleAlbumArtistFilter, inSubtree})
		var chunkIDs []string
		if err := r.queryAllSlice(ctx, sq, &chunkIDs); err != nil {
			return nil, err
		}
		ids = append(ids, chunkIDs...)
	}
	return ids, nil
}

func (r *albumRepository) GetCursor(ctx context.Context, options ...model.QueryOptions) (model.AlbumCursor, error) {
	ids, err := r.getAllIDs(ctx, options...)
	if err != nil {
		return nil, err
	}
	opts := chunkOptions(options, "album.id")
	return model.AlbumCursor(streamByIDs(ids, func(chunk []string) (model.Albums, error) {
		return r.GetAll(ctx, opts(chunk))
	})), nil
}

func (r *albumRepository) GetYears(ctx context.Context, libraryIDs ...int) ([]int, error) {
	cond := And{Gt{"max_year": 0}, Eq{"missing": false}}
	if len(libraryIDs) > 0 {
		cond = append(cond, Eq{"library_id": libraryIDs})
	}
	sq := r.applyLibraryFilter(ctx, Select("distinct max_year").From("album").Where(cond).OrderBy("max_year"))
	years := []int{}
	err := r.queryAllSlice(ctx, sq, &years)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}
	return years, nil
}

func (r *albumRepository) CopyAttributes(ctx context.Context, fromID, toID string, columns ...string) error {
	// Cast values to text so go-sqlite3 does not decode datetime columns as time.Time
	// and reformat them as RFC3339 when written back.
	sel := slice.Map(columns, func(c string) string { return fmt.Sprintf("cast(%[1]s as text) as %[1]s", c) })
	var from dbx.NullStringMap
	err := r.queryOne(ctx, Select(sel...).From(r.tableName).Where(Eq{"id": fromID}), &from)
	if err != nil {
		return fmt.Errorf("getting album to copy fields from: %w", err)
	}
	to := make(map[string]any)
	for _, col := range columns {
		v := from[col]
		// created_at is aggregated from song birth_times and must never be
		// overwritten with a zero/poisoned value, or it propagates forward on
		// every metadata-driven album ID change.
		if col == "created_at" && (!v.Valid || v.String == "" || strings.HasPrefix(v.String, "0001-")) {
			continue
		}
		to[col] = v
	}
	if len(to) == 0 {
		return nil
	}
	_, err = r.executeSQL(ctx, Update(r.tableName).SetMap(to).Where(Eq{"id": toID}))
	return err
}

// Touch flags an album as being scanned by the scanner, but not necessarily updated.
// This is used for when missing tracks are detected for an album during scan.
func (r *albumRepository) Touch(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	for ids := range slices.Chunk(ids, 200) {
		upd := Update(r.tableName).Set("imported_at", time.Now()).Where(Eq{"id": ids})
		c, err := r.executeSQL(ctx, upd)
		if err != nil {
			return fmt.Errorf("error touching albums: %w", err)
		}
		log.Debug(ctx, "Touching albums", "ids", ids, "updated", c)
	}
	return nil
}

// TouchByMissingFolder touches all albums that have missing folders
func (r *albumRepository) TouchByMissingFolder(ctx context.Context) (int64, error) {
	upd := Update(r.tableName).Set("imported_at", time.Now()).
		Where(And{
			NotEq{"folder_ids": nil},
			ConcatExpr("EXISTS (SELECT 1 FROM json_each(folder_ids) AS je JOIN main.folder AS f ON je.value = f.id WHERE f.missing = true)"),
		})
	c, err := r.executeSQL(ctx, upd)
	if err != nil {
		return 0, fmt.Errorf("error touching albums by missing folder: %w", err)
	}
	return c, nil
}

// GetTouchedAlbums returns all albums that were touched by the scanner for a given library, in the
// current library scan run.
// It does not need to load participants, as they are not used by the scanner.
func (r *albumRepository) GetTouchedAlbums(ctx context.Context, libID int) (model.AlbumCursor, error) {
	query := r.selectAlbum(ctx).
		Where(And{
			Eq{"library.id": libID},
			ConcatExpr("album.imported_at > library.last_scan_at"),
		})
	cursor, err := queryWithStableResults[dbAlbum](ctx, r.sqlRepository, query)
	if err != nil {
		return nil, err
	}
	return wrapAlbumCursor(cursor), nil
}

func wrapAlbumCursor(cursor iter.Seq2[dbAlbum, error]) model.AlbumCursor {
	return model.AlbumCursor(wrapCursor(cursor, func(a dbAlbum) *model.Album { return a.Album }))
}

// RefreshPlayCounts updates the play count and last play date annotations for all albums, based
// on the media files associated with them.
func (r *albumRepository) RefreshPlayCounts(ctx context.Context) (int64, error) {
	query := Expr(`
with play_counts as (
    select user_id, album_id, sum(play_count) as total_play_count, max(play_date) as last_play_date
    from media_file
             join annotation on item_id = media_file.id
    group by user_id, album_id
)
insert into annotation (user_id, item_id, item_type, play_count, play_date)
select user_id, album_id, 'album', total_play_count, last_play_date
from play_counts
where total_play_count > 0
on conflict (user_id, item_id, item_type) do update
    set play_count = excluded.play_count,
        play_date  = excluded.play_date;
`)
	return r.executeSQL(ctx, query)
}

func (r *albumRepository) purgeEmpty(ctx context.Context, libraryIDs ...int) error {
	del := Delete(r.tableName).Where("id not in (select distinct(album_id) from media_file)")
	// If libraryIDs are specified, only purge albums from those libraries
	if len(libraryIDs) > 0 {
		del = del.Where(Eq{"library_id": libraryIDs})
	}
	c, err := r.executeSQL(ctx, del)
	if err != nil {
		return fmt.Errorf("purging empty albums: %w", err)
	}
	if c > 0 {
		log.Debug(ctx, "Purged empty albums", "totalDeleted", c)
	}
	return nil
}

var albumSearchConfig = searchConfig{
	NaturalOrder: "album.rowid",
	OrderBy:      []string{"name"},
	MBIDFields:   []string{"mbz_album_id", "mbz_release_group_id"},
}

func (r *albumRepository) Search(ctx context.Context, q string, options ...model.QueryOptions) (model.Albums, error) {
	var opts model.QueryOptions
	if len(options) > 0 {
		opts = options[0]
	}
	var res dbAlbums
	err := r.doSearch(ctx, r.selectAlbum(ctx, options...), q, &res, albumSearchConfig, opts)
	if err != nil {
		return nil, fmt.Errorf("searching album %q: %w", q, err)
	}
	albums := res.toModels()
	r.hydrateArtwork(ctx, albums)
	return albums, nil
}

func (r *albumRepository) Count(ctx context.Context, options ...rest.QueryOptions) (int64, error) {
	return r.CountAll(ctx, r.parseRestOptions(ctx, options...))
}

func (r *albumRepository) Read(ctx context.Context, id string) (*model.Album, error) {
	return r.Get(ctx, id)
}

func (r *albumRepository) ReadAll(ctx context.Context, options ...rest.QueryOptions) ([]model.Album, error) {
	return r.GetAll(ctx, r.parseRestOptions(ctx, options...))
}

var _ model.AlbumRepository = (*albumRepository)(nil)
var _ rest.Repository[model.Album] = (*albumRepository)(nil)
