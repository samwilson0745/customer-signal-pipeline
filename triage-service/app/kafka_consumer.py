import json
import logging
import threading
import time

from kafka import KafkaConsumer, KafkaProducer
from kafka.errors import KafkaError

from app import cassandra_client, es_client, redis_client
from app.classifier import ClassificationError, classify
from app.config import settings
from app.models import EnrichedEvent, RawEvent

logger = logging.getLogger("triage.consumer")

_stop_event = threading.Event()
_dlq_producer: KafkaProducer | None = None


def _get_dlq_producer() -> KafkaProducer:
    global _dlq_producer
    if _dlq_producer is None:
        _dlq_producer = KafkaProducer(
            bootstrap_servers=settings.kafka_brokers,
            value_serializer=lambda v: json.dumps(v).encode("utf-8"),
        )
    return _dlq_producer


def _send_to_dlq(raw_value: bytes, reason: str) -> None:
    try:
        payload = {
            "reason": reason,
            "raw_value": raw_value.decode("utf-8", errors="replace"),
            "failed_at": time.time(),
        }
        producer = _get_dlq_producer()
        producer.send(settings.kafka_dlq_topic, value=payload)
        producer.flush(timeout=5)
        logger.error("event sent to DLQ topic=%s reason=%s", settings.kafka_dlq_topic, reason)
    except KafkaError:
        logger.exception("failed to publish to DLQ, logging inline instead: %s", reason)


def _build_consumer() -> KafkaConsumer:
    last_error: Exception | None = None
    for attempt in range(1, 11):
        try:
            return KafkaConsumer(
                settings.kafka_topic,
                bootstrap_servers=settings.kafka_brokers,
                group_id=settings.kafka_consumer_group,
                auto_offset_reset="earliest",
                enable_auto_commit=True,
                value_deserializer=lambda v: v,
                consumer_timeout_ms=1000,
            )
        except KafkaError as exc:
            last_error = exc
            logger.warning("kafka consumer connect attempt %d failed: %s", attempt, exc)
            time.sleep(min(2 ** attempt, 20))
    raise RuntimeError(f"could not connect to kafka after retries: {last_error}")


def process_message(raw_value: bytes) -> None:
    try:
        payload = json.loads(raw_value.decode("utf-8"))
        event = RawEvent.model_validate(payload)
    except Exception as exc:  # noqa: BLE001
        logger.error("malformed event, sending to DLQ: %s", exc)
        _send_to_dlq(raw_value, f"malformed_event: {exc}")
        return

    try:
        classification, source = classify(event)
    except ClassificationError as exc:
        logger.error("classification failed for event_id=%s after retries: %s", event.event_id, exc)
        _send_to_dlq(raw_value, f"classification_failed: {exc}")
        return

    enriched = EnrichedEvent(
        **event.model_dump(),
        sentiment=classification.sentiment,
        urgency=classification.urgency,
        topic=classification.topic,
        classifier_source=source,
    )

    try:
        cassandra_client.insert_raw_event(event)
        es_client.index_enriched_event(enriched)
        redis_client.increment_stats(enriched)
        logger.info(
            "processed event_id=%s brand=%s sentiment=%s urgency=%s topic=%s source=%s",
            event.event_id, event.brand, classification.sentiment, classification.urgency, classification.topic, source,
        )
    except Exception as exc:  # noqa: BLE001 - downstream writes exhausted their own retries
        logger.error("downstream write failed for event_id=%s after retries: %s", event.event_id, exc)
        _send_to_dlq(raw_value, f"downstream_write_failed: {exc}")


def run_consumer_loop() -> None:
    logger.info("starting kafka consumer loop: topic=%s group=%s", settings.kafka_topic, settings.kafka_consumer_group)
    consumer = _build_consumer()
    while not _stop_event.is_set():
        for message in consumer:
            if _stop_event.is_set():
                break
            process_message(message.value)
    consumer.close()
    logger.info("kafka consumer loop stopped")


def start_background_consumer() -> threading.Thread:
    thread = threading.Thread(target=run_consumer_loop, name="kafka-consumer", daemon=True)
    thread.start()
    return thread


def stop_background_consumer() -> None:
    _stop_event.set()
    _close_dlq_producer()


def _close_dlq_producer() -> None:
    global _dlq_producer
    if _dlq_producer is None:
        return
    try:
        _dlq_producer.flush(timeout=5)
        _dlq_producer.close(timeout=5)
    except KafkaError:
        logger.exception("error closing DLQ producer")
    finally:
        _dlq_producer = None
