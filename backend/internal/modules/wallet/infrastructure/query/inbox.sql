-- name: MarkMessageProcessed :execrows
-- Records a successfully consumed message. ON CONFLICT DO NOTHING makes a
-- redelivery a no-op; 0 rows affected means the message was already processed.
INSERT INTO inbox_events (consumer_group, message_key)
VALUES ($1, $2)
ON CONFLICT (consumer_group, message_key) DO NOTHING;

-- name: IsMessageProcessed :one
SELECT EXISTS (
    SELECT 1
    FROM inbox_events
    WHERE consumer_group = $1 AND message_key = $2
);
