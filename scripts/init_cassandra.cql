-- Reference copy of the schema the triage-service applies automatically on
-- startup (see triage-service/app/cassandra_client.py). Kept here so the
-- schema is reviewable/runnable by hand, e.g.:
--   docker compose exec cassandra cqlsh -f /scripts/init_cassandra.cql

CREATE KEYSPACE IF NOT EXISTS customer_signals
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE customer_signals;

CREATE TABLE IF NOT EXISTS raw_events (
  brand text,
  created_at timestamp,
  event_id text,
  channel text,
  author text,
  text text,
  lang text,
  PRIMARY KEY ((brand), created_at, event_id)
) WITH CLUSTERING ORDER BY (created_at DESC);
