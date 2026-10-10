-- +goose Up
alter table album add column is_audiobook integer not null default 0;
create index album_is_audiobook_library on album(is_audiobook, library_id);

create table album_audiobook_override (
    album_id varchar primary key references album(id) on delete cascade,
    is_audiobook integer not null check(is_audiobook in (0, 1))
);

create table audiobook_progress (
    album_id varchar not null references album(id) on delete cascade,
    user_id varchar not null references user(id) on delete cascade,
    track_id varchar not null,
    position_ms integer not null default 0,
    completed integer not null default 0,
    updated_at datetime not null default CURRENT_TIMESTAMP,
    primary key(album_id, user_id)
);

-- +goose Down
drop table audiobook_progress;
drop table album_audiobook_override;
drop index album_is_audiobook_library;
alter table album drop column is_audiobook;
