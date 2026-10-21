-- +goose Up
-- JSON of domain.VersionJudgeEvidence: the frame rule's counts and the frames shown to the classifier.
alter table activity_entries add column evidence text;

-- +goose Down
alter table activity_entries drop column evidence;
