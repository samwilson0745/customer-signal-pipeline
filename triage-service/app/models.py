from datetime import datetime, timezone
from typing import Literal, Optional

from pydantic import BaseModel, Field, field_validator

Sentiment = Literal["positive", "neutral", "negative"]
Urgency = Literal["low", "medium", "high", "critical"]


class RawEvent(BaseModel):
    event_id: str
    channel: Literal["twitter", "chat", "email", "app_review"]
    author: str
    text: str
    lang: str = "en"
    created_at: datetime
    brand: str

    @field_validator("created_at", mode="before")
    @classmethod
    def parse_created_at(cls, value: object) -> object:
        if isinstance(value, str):
            # Support both "...Z" and offset-qualified ISO-8601.
            return datetime.fromisoformat(value.replace("Z", "+00:00"))
        return value


class Classification(BaseModel):
    sentiment: Sentiment
    urgency: Urgency
    topic: str = Field(min_length=1, max_length=64)


class EnrichedEvent(RawEvent):
    sentiment: Sentiment
    urgency: Urgency
    topic: str
    classified_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    classifier_source: Optional[str] = None
