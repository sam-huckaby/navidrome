-- +goose Up
ALTER TABLE media_file ADD COLUMN chapter_data TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE media_file DROP COLUMN chapter_data;
