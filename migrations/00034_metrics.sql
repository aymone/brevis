-- +goose Up
-- The live volume of gateways and SQL APIs (stream health, Endpoints), kept in
-- the database so every API process answers the same thing. One collector (the
-- scheduler, behind an advisory lock) writes these two tables; the API only
-- reads them. New tables only, nothing renamed: a rolling update is safe.
CREATE TABLE metric_sources (
    kind             TEXT             NOT NULL,
    owner            TEXT             NOT NULL,
    instance         TEXT             NOT NULL,
    started_at       TIMESTAMPTZ      NOT NULL,
    interval_seconds DOUBLE PRECISION NOT NULL,
    last_try         TIMESTAMPTZ,
    last_ok          TIMESTAMPTZ,
    last_error       TEXT             NOT NULL DEFAULT '',
    diffs            INTEGER          NOT NULL DEFAULT 0,
    dropped          INTEGER          NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, owner, instance)
);

COMMENT ON TABLE metric_sources IS
  'The pods the collector found by annotation, one row per replica. updated_at is its heartbeat: a row not touched for a few scrape intervals is a collector that stopped, and the console says there is no source.';
COMMENT ON COLUMN metric_sources.kind IS 'gateway or endpoints.';
COMMENT ON COLUMN metric_sources.owner IS 'The gateway''s name: or the API''s name, as the console knows it.';
COMMENT ON COLUMN metric_sources.instance IS 'The pod''s address (host:port); never a credential.';
COMMENT ON COLUMN metric_sources.last_error IS 'A class (timeout, status, unreachable...), never the error''s text.';

CREATE TABLE metric_minutes (
    kind            TEXT             NOT NULL,
    owner           TEXT             NOT NULL,
    entity          TEXT             NOT NULL,
    minute          BIGINT           NOT NULL,
    counts          JSONB            NOT NULL,
    latency         JSONB            NOT NULL DEFAULT '{}',
    covered_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
    gauges          JSONB,
    rate            JSONB,
    updated_at      TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, owner, entity, minute)
);

CREATE INDEX metric_minutes_minute_idx ON metric_minutes (minute);
CREATE INDEX metric_minutes_updated_idx ON metric_minutes (kind, updated_at DESC);

COMMENT ON TABLE metric_minutes IS
  'Per-minute counters of a stream (gateway) or an endpoint (SQL API), summed over the owner''s replicas. Rows older than BREVIS_RETENTION_METRICS_HOURS are pruned by the collector.';
COMMENT ON COLUMN metric_minutes.minute IS 'Unix time divided by 60.';
COMMENT ON COLUMN metric_minutes.entity IS 'The stream or endpoint name; the empty string is the owner''s own row, which carries only covered_seconds.';
COMMENT ON COLUMN metric_minutes.counts IS 'Increase over the minute, one number per measure of the kind, in the order of the measures list.';
COMMENT ON COLUMN metric_minutes.latency IS 'Increase of each cumulative histogram bucket over the minute, keyed by its upper bound (le).';
COMMENT ON COLUMN metric_minutes.covered_seconds IS 'Seconds of the minute the owner was actually observed (best replica).';
COMMENT ON COLUMN metric_minutes.gauges IS 'Current gauges (buffered, queued), summed over answering replicas; only on the newest row of an entity.';
COMMENT ON COLUMN metric_minutes.rate IS 'Events per minute over the last minute of scrapes; only on the newest row of an entity.';

-- +goose Down
DROP TABLE metric_minutes;
DROP TABLE metric_sources;
