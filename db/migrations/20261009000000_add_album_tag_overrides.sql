-- +goose Up
create table album_tag_override (
    album_id varchar primary key references album(id) on delete cascade,
    tags text not null
);

-- +goose Down
drop table album_tag_override;
