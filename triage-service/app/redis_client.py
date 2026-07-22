import logging

import redis
from tenacity import retry, stop_after_attempt, wait_exponential

from app.config import settings
from app.models import EnrichedEvent

logger = logging.getLogger("triage.redis")

_client: redis.Redis | None = None


def get_client() -> redis.Redis:
    global _client
    if _client is None:
        _client = redis.from_url(settings.redis_url, decode_responses=True)
    return _client


def ping() -> bool:
    """Lightweight liveness check for /health. Does not attempt to
    (re)connect; just reports whether the existing client (if any) responds."""
    if _client is None:
        return False
    try:
        return bool(_client.ping())
    except Exception:  # noqa: BLE001
        return False


@retry(reraise=True, stop=stop_after_attempt(5), wait=wait_exponential(multiplier=0.5, min=0.5, max=8))
def increment_stats(event: EnrichedEvent) -> None:
    """Bump the live sentiment/urgency counters for a brand.

    Uses a rolling TTL: every increment refreshes the expiry so a key only
    goes cold `STATS_TTL_SECONDS` after the *last* matching event, giving the
    Query API a near-real-time window without unbounded growth in Redis.
    """
    client = get_client()
    sentiment_key = f"stats:{event.brand}:sentiment:{event.sentiment}"
    urgency_key = f"stats:{event.brand}:urgency:{event.urgency}"
    total_key = f"stats:{event.brand}:total"

    pipe = client.pipeline()
    pipe.incr(sentiment_key)
    pipe.expire(sentiment_key, settings.stats_ttl_seconds)
    pipe.incr(urgency_key)
    pipe.expire(urgency_key, settings.stats_ttl_seconds)
    pipe.incr(total_key)
    pipe.expire(total_key, settings.stats_ttl_seconds)
    pipe.execute()
