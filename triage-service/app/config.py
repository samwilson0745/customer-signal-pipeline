import os
from dataclasses import dataclass, field
from typing import List


def _split_csv(value: str) -> List[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


@dataclass
class Settings:
    kafka_brokers: List[str] = field(default_factory=lambda: _split_csv(os.getenv("KAFKA_BROKERS", "localhost:9092")))
    kafka_topic: str = os.getenv("KAFKA_TOPIC", "customer-signals")
    kafka_dlq_topic: str = os.getenv("KAFKA_DLQ_TOPIC", "customer-signals-dlq")
    kafka_consumer_group: str = os.getenv("KAFKA_CONSUMER_GROUP", "triage-service")

    gemini_api_key: str = os.getenv("GEMINI_API_KEY", "")
    gemini_model: str = os.getenv("GEMINI_MODEL", "gemini-2.0-flash")

    cassandra_hosts: List[str] = field(default_factory=lambda: _split_csv(os.getenv("CASSANDRA_HOSTS", "localhost")))
    cassandra_port: int = int(os.getenv("CASSANDRA_PORT", "9042"))
    cassandra_keyspace: str = os.getenv("CASSANDRA_KEYSPACE", "customer_signals")

    es_url: str = os.getenv("ES_URL", "http://localhost:9200")
    es_index: str = os.getenv("ES_INDEX", "customer-signals")

    redis_url: str = os.getenv("REDIS_URL", "redis://localhost:6379/0")
    stats_ttl_seconds: int = int(os.getenv("STATS_TTL_SECONDS", "86400"))

    health_port: int = int(os.getenv("TRIAGE_HEALTH_PORT", "8000"))

    max_classify_retries: int = int(os.getenv("MAX_CLASSIFY_RETRIES", "3"))
    max_write_retries: int = int(os.getenv("MAX_WRITE_RETRIES", "5"))


settings = Settings()
