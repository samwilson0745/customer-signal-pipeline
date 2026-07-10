import logging

from cassandra.cluster import Cluster, Session
from tenacity import retry, stop_after_attempt, wait_exponential

from app.config import settings
from app.models import RawEvent

logger = logging.getLogger("triage.cassandra")

_session: Session | None = None

CREATE_KEYSPACE = """
CREATE KEYSPACE IF NOT EXISTS {keyspace}
WITH replication = {{'class': 'SimpleStrategy', 'replication_factor': 1}}
"""

CREATE_TABLE = """
CREATE TABLE IF NOT EXISTS raw_events (
  brand text,
  created_at timestamp,
  event_id text,
  channel text,
  author text,
  text text,
  lang text,
  PRIMARY KEY ((brand), created_at, event_id)
) WITH CLUSTERING ORDER BY (created_at DESC)
"""

INSERT_RAW_EVENT = """
INSERT INTO raw_events (brand, created_at, event_id, channel, author, text, lang)
VALUES (%s, %s, %s, %s, %s, %s, %s)
"""


@retry(stop=stop_after_attempt(10), wait=wait_exponential(multiplier=1, min=2, max=20))
def get_session() -> Session:
    global _session
    if _session is not None:
        return _session

    cluster = Cluster(settings.cassandra_hosts, port=settings.cassandra_port)
    session = cluster.connect()
    session.execute(CREATE_KEYSPACE.format(keyspace=settings.cassandra_keyspace))
    session.set_keyspace(settings.cassandra_keyspace)
    session.execute(CREATE_TABLE)
    logger.info("cassandra keyspace/table ready: %s", settings.cassandra_keyspace)
    _session = session
    return _session


@retry(reraise=True, stop=stop_after_attempt(5), wait=wait_exponential(multiplier=0.5, min=0.5, max=8))
def insert_raw_event(event: RawEvent) -> None:
    session = get_session()
    session.execute(
        INSERT_RAW_EVENT,
        (event.brand, event.created_at, event.event_id, event.channel, event.author, event.text, event.lang),
    )
