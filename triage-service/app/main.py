import logging

from fastapi import FastAPI, Response

from app.config import settings
from app.kafka_consumer import start_background_consumer, stop_background_consumer

logging.basicConfig(
    level=logging.INFO,
    format='{"level":"%(levelname)s","service":"triage-service","logger":"%(name)s","msg":"%(message)s","ts":"%(asctime)s"}',
)
logger = logging.getLogger("triage.main")

app = FastAPI(title="triage-service", version="1.0.0")

_consumer_thread = None


@app.on_event("startup")
def on_startup() -> None:
    global _consumer_thread
    logger.info("triage-service starting up, kafka_topic=%s", settings.kafka_topic)
    _consumer_thread = start_background_consumer()


@app.on_event("shutdown")
def on_shutdown() -> None:
    logger.info("triage-service shutting down")
    stop_background_consumer()


@app.get("/health")
def health(response: Response) -> dict:
    consumer_alive = _consumer_thread is not None and _consumer_thread.is_alive()
    if not consumer_alive:
        response.status_code = 503
    return {
        "status": "ok" if consumer_alive else "degraded",
        "kafka_consumer_alive": consumer_alive,
    }


@app.get("/")
def root() -> dict:
    return {"service": "triage-service", "status": "running"}
