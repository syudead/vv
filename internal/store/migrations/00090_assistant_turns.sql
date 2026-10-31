-- +goose Up
-- Turns of the assistant's tool loop: one request run as a server-side job. The transcript stays in memory; the
-- row keeps what the panel shows. previous_turn_id and proposal_id have no foreign keys, since each side expires on
-- its own schedule.
create table assistant_turns (
    id               integer primary key autoincrement,
    status           text    not null check (status in ('running', 'done', 'clarify', 'stopped', 'interrupted')),
    request          text    not null,
    -- domain.AssistantTurnContext as JSON.
    context          text    not null,
    previous_turn_id integer,
    -- [domain.AssistantProgressItem] as JSON, in call order.
    progress         text    not null default '[]',
    answer           text,
    judged           integer not null default 0 check (judged in (0, 1)),
    -- domain.AssistantDirective, domain.AssistantTurnClarification and domain.AssistantTurnStop as JSON.
    directive        text,
    clarify          text,
    proposal_id      integer,
    stop             text,
    -- Unix milliseconds.
    started_at       integer not null,
    finished_at      integer
);
-- One local model serves one request at a time.
create unique index assistant_turns_running_idx on assistant_turns (status) where status = 'running';
create index assistant_turns_finished_idx on assistant_turns (finished_at);

-- +goose Down
drop table assistant_turns;
