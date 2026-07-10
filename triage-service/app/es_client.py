import logging

from elasticsearch import Elasticsearch
from tenacity import retry, stop_after_attempt, wait_exponential

from app.config import settings
from app.models import EnrichedEvent

logger = logging.getLogger("triage.elasticsearch")

_client: Elasticsearch | None = None

INDEX_MAPPING = {
    "mappings": {
        "properties": {
            "event_id": {"type": "keyword"},
            "channel": {"type": "keyword"},
            "author": {"type": "keyword"},
            "text": {"type": "text"},
            "lang": {"type": "keyword"},
            "brand": {"type": "keyword"},
            "sentiment": {"type": "keyword"},
            "urgency": {"type": "keyword"},
            "topic": {"type": "keyword"},
            "created_at": {"type": "date"},
            "classified_at": {"type": "date"},
            "classifier_source": {"type": "keyword"},
        }
    }
}


@retry(stop=stop_after_attempt(10), wait=wait_exponential(multiplier=1, min=2, max=20))
def get_client() -> Elasticsearch:
    global _client
    if _client is not None:
        return _client

    client = Elasticsearch(settings.es_url)
    if not client.indices.exists(index=settings.es_index):
        client.indices.create(index=settings.es_index, body=INDEX_MAPPING)
        logger.info("created elasticsearch index: %s", settings.es_index)
    _client = client
    return _client


@retry(reraise=True, stop=stop_after_attempt(5), wait=wait_exponential(multiplier=0.5, min=0.5, max=8))
def index_enriched_event(event: EnrichedEvent) -> None:
    client = get_client()
    client.index(index=settings.es_index, id=event.event_id, document=event.model_dump(mode="json"))
