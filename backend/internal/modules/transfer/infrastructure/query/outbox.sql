-- name: InsertOutboxEvent :one
-- Staged in the same transaction as the state change it describes; iris (CDC)
-- drains outbox_events to Kafka via logical replication (no application poll).
INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
    VALUES ($1, $2, $3, $4)
RETURNING
    *;

