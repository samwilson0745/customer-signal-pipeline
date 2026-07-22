from unittest.mock import MagicMock

from app import kafka_consumer


def test_stop_background_consumer_sets_stop_event_and_closes_producer(monkeypatch):
    monkeypatch.setattr(kafka_consumer, "_stop_event", MagicMock())
    producer = MagicMock()
    monkeypatch.setattr(kafka_consumer, "_dlq_producer", producer)

    kafka_consumer.stop_background_consumer()

    kafka_consumer._stop_event.set.assert_called_once()
    producer.flush.assert_called_once()
    producer.close.assert_called_once()
    assert kafka_consumer._dlq_producer is None


def test_close_dlq_producer_is_a_noop_when_never_created(monkeypatch):
    monkeypatch.setattr(kafka_consumer, "_dlq_producer", None)
    kafka_consumer._close_dlq_producer()  # should not raise
    assert kafka_consumer._dlq_producer is None
