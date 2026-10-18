-- +goose Up
-- Profile fields are nullable so existing people start with an empty profile; domain.NormalizePersonProfile checks values.
alter table people add column gender text;
alter table people add column birth_date text;
alter table people add column death_date text;
alter table people add column birthplace text;
alter table people add column height_cm integer;
alter table people add column bust_cm integer;
alter table people add column waist_cm integer;
alter table people add column hips_cm integer;
alter table people add column cup text;
alter table people add column blood_type text;
alter table people add column career_start_year integer;
alter table people add column career_end_year integer;
alter table people add column bio text;
create table people_links (
    person_id integer not null references people (id) on delete cascade,
    position  integer not null,
    url       text    not null,
    primary key (person_id, position),
    unique (person_id, url)
) without rowid;

-- No smart rule or collection value reads a profile, so profile updates must not queue an all-folder refresh.
drop trigger people_collection_u;
-- +goose StatementBegin
create trigger people_collection_u after update of id, created_at on people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd

-- +goose Down
drop trigger people_collection_u;
-- +goose StatementBegin
create trigger people_collection_u after update on people begin
    insert into collection_changes values ('smart', 0);
end;
-- +goose StatementEnd
drop table people_links;
alter table people drop column bio;
alter table people drop column career_end_year;
alter table people drop column career_start_year;
alter table people drop column blood_type;
alter table people drop column cup;
alter table people drop column hips_cm;
alter table people drop column waist_cm;
alter table people drop column bust_cm;
alter table people drop column height_cm;
alter table people drop column birthplace;
alter table people drop column death_date;
alter table people drop column birth_date;
alter table people drop column gender;
