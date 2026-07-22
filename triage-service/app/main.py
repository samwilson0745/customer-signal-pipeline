import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Response

from app import cassandra_client, es_client, redis_client
from app.config import settings
from app.kafka_consumer import start_background_consumer, stop_background_consumer

logging.basicConfig(
    level=logging.INFO,
    format='{"level":"%(levelname)s","service":"triage-service","logger":"%(name)s","msg":"%(message)s","ts":"%(asctime)s"}',
)
logger = logging.getLogger("triage.main")

_consumer_thread = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _consumer_thread
    logger.info("triage-service starting up, kafka_topic=%s", settings.kafka_topic)
    _consumer_thread = start_background_consumer()
    yield
    logger.info("triage-service shutting down")
    stop_background_consumer()


app = FastAPI(title="triage-service", version="1.0.0", lifespan=lifespan)


@app.get("/health")
def health(response: Response) -> dict:
    """Reports consumer-thread liveness plus each downstream store's
    reachability. Note: a store shows unhealthy until this process has made
    its first successful call to it (clients connect lazily), so "degraded"
    immediately after startup with no traffic yet is expected, not a bug."""
    consumer_alive = _consumer_thread is not None and _consumer_thread.is_alive()
    deps = {
        "cassandra": cassandra_client.ping(),
        "elasticsearch": es_client.ping(),
        "redis": redis_client.ping(),
    }
    healthy = consumer_alive and all(deps.values())
    if not healthy:
        response.status_code = 503
    return {
        "status": "ok" if healthy else "degraded",
        "kafka_consumer_alive": consumer_alive,
        **deps,
    }


@app.get("/")
def root() -> dict:
    return {"service": "triage-service", "status": "running"}
