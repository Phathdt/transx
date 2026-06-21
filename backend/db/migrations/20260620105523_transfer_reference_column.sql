-- +goose Up
-- +goose StatementBegin
-- reference: business id ETN-/ITN- + ULID. The app fills new rows at INSERT time
-- (prefix from transfer_type + ulid.Make()). This migration backfills existing
-- dev rows with a ULID-shaped value derived from created_at so no app run is
-- needed before SET NOT NULL.
ALTER TABLE transfers
    ADD COLUMN reference TEXT;

-- Backfill: 26-char Crockford base32 ULID-like (48-bit time + 80-bit random).
DO $$
DECLARE
    r RECORD;
    alphabet CONSTANT text := '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
    body text;
    i int;
    n bigint;
BEGIN
    FOR r IN
    SELECT
        id,
        transfer_type,
        created_at
    FROM
        transfers
    WHERE
        reference IS NULL LOOP
            body := '';
            -- 10 chars timestamp (encodes the 48-bit millisecond component).
            n := floor(extract(epoch FROM r.created_at) * 1000)::bigint;
            FOR i IN 1..10 LOOP
                body := substr(alphabet, (n % 32)::int + 1, 1) || body;
                n := n / 32;
            END LOOP;
            -- 16 chars random (stand-in for the 80-bit entropy component).
            FOR i IN 1..16 LOOP
                body := body || substr(alphabet, (floor(random() * 32))::int + 1, 1);
            END LOOP;
            UPDATE
                transfers
            SET
                reference = (
                    CASE WHEN r.transfer_type = 'EXTERNAL' THEN
                        'ETN-'
                    ELSE
                        'ITN-'
                    END) || body
            WHERE
                id = r.id;
        END LOOP;
END
$$;

ALTER TABLE transfers
    ALTER COLUMN reference SET NOT NULL;

CREATE UNIQUE INDEX uq_transfers_reference ON transfers (reference);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP INDEX uq_transfers_reference;

ALTER TABLE transfers
    DROP COLUMN reference;

-- +goose StatementEnd
