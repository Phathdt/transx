-- +goose Up
-- +goose StatementBegin
-- iris (CDC publisher) reads outbox_events via Postgres logical replication and
-- requires a publication named exactly "pglogrepl_publication" (the name is
-- hardcoded in iris v0.1.0). Scope it to outbox_events only so the CDC stream
-- carries just the transfer outbox, not every table.
CREATE PUBLICATION pglogrepl_publication FOR TABLE outbox_events;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP PUBLICATION IF EXISTS pglogrepl_publication;

-- +goose StatementEnd
