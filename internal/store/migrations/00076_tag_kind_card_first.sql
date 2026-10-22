-- +goose Up
-- Cards list tags of flagged kinds first. The seeded genre kind starts flagged under its English or Japanese name.
alter table tag_kinds add column card_first integer not null default 0;
update tag_kinds set card_first = 1 where name in ('Genre', 'ジャンル');

-- +goose Down
alter table tag_kinds drop column card_first;
