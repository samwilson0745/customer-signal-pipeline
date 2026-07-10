from datetime import datetime, timezone
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest

from app.classifier import (
    ClassificationError,
    classify,
    classify_heuristic,
    classify_with_llm,
    parse_classification,
)
from app.models import RawEvent


def make_event(text: str, channel: str = "twitter", brand: str = "acme") -> RawEvent:
    return RawEvent(
        event_id="evt_1",
        channel=channel,
        author="alex123",
        text=text,
        lang="en",
        created_at=datetime.now(timezone.utc),
        brand=brand,
    )


class TestParseClassification:
    def test_parses_clean_json(self):
        result = parse_classification('{"sentiment": "negative", "urgency": "high", "topic": "billing"}')
        assert result.sentiment == "negative"
        assert result.urgency == "high"
        assert result.topic == "billing"

    def test_strips_markdown_fences_and_prose(self):
        raw = 'Sure, here is the classification:\n```json\n{"sentiment": "positive", "urgency": "low", "topic": "praise"}\n```\nLet me know if you need anything else.'
        result = parse_classification(raw)
        assert result.sentiment == "positive"
        assert result.urgency == "low"
        assert result.topic == "praise"

    def test_normalizes_case_and_whitespace_in_topic(self):
        result = parse_classification('{"sentiment": "NEUTRAL", "urgency": "Medium", "topic": "Feature Request"}')
        assert result.sentiment == "neutral"
        assert result.urgency == "medium"
        assert result.topic == "feature_request"

    def test_maps_sentiment_synonyms(self):
        result = parse_classification('{"sentiment": "angry", "urgency": "low", "topic": "general"}')
        assert result.sentiment == "negative"

    def test_maps_urgency_synonyms(self):
        result = parse_classification('{"sentiment": "neutral", "urgency": "emergency", "topic": "outage"}')
        assert result.urgency == "critical"

    def test_defaults_missing_topic_to_general(self):
        result = parse_classification('{"sentiment": "positive", "urgency": "low"}')
        assert result.topic == "general"

    def test_raises_on_empty_text(self):
        with pytest.raises(ValueError):
            parse_classification("")

    def test_raises_on_non_json_text(self):
        with pytest.raises(ValueError):
            parse_classification("I cannot classify this message.")

    def test_raises_on_invalid_sentiment(self):
        with pytest.raises(ValueError):
            parse_classification('{"sentiment": "ecstatic", "urgency": "low", "topic": "general"}')

    def test_raises_on_invalid_urgency(self):
        with pytest.raises(ValueError):
            parse_classification('{"sentiment": "positive", "urgency": "whenever", "topic": "general"}')


class TestClassifyWithLLM:
    def test_returns_classification_on_first_success(self):
        mock_llm = MagicMock()
        mock_llm.invoke.return_value = SimpleNamespace(
            content='{"sentiment": "negative", "urgency": "critical", "topic": "outage"}'
        )
        event = make_event("The site has been down for an hour!")
        result = classify_with_llm(event, llm=mock_llm)
        assert result.sentiment == "negative"
        assert result.urgency == "critical"
        assert mock_llm.invoke.call_count == 1

    def test_retries_on_malformed_output_then_succeeds(self):
        mock_llm = MagicMock()
        mock_llm.invoke.side_effect = [
            SimpleNamespace(content="not json at all"),
            SimpleNamespace(content='{"sentiment": "positive", "urgency": "low", "topic": "praise"}'),
        ]
        event = make_event("Loving the new dashboard!")
        result = classify_with_llm(event, llm=mock_llm)
        assert result.sentiment == "positive"
        assert mock_llm.invoke.call_count == 2

    def test_exhausts_retries_and_raises(self):
        mock_llm = MagicMock()
        mock_llm.invoke.return_value = SimpleNamespace(content="garbage")
        event = make_event("???")
        with pytest.raises(ValueError):
            classify_with_llm(event, llm=mock_llm)
        assert mock_llm.invoke.call_count == 3  # stop_after_attempt(3)


class TestClassifyHeuristic:
    def test_detects_negative_urgent_outage(self):
        event = make_event("URGENT: the app has been down for 45 minutes, fix this now.")
        result = classify_heuristic(event)
        assert result.urgency == "critical"
        assert result.topic == "outage"

    def test_detects_positive_praise(self):
        event = make_event("Thanks so much, your team is amazing, really appreciate the quick fix!")
        result = classify_heuristic(event)
        assert result.sentiment == "positive"
        assert result.topic == "praise"

    def test_detects_billing_topic(self):
        event = make_event("I was charged twice this month, please issue a refund.")
        result = classify_heuristic(event)
        assert result.topic == "billing"

    def test_defaults_to_neutral_general(self):
        event = make_event("What's the difference between the Pro and Enterprise plans?")
        result = classify_heuristic(event)
        assert result.sentiment == "neutral"
        assert result.topic == "general"


class TestClassifyDispatch:
    def test_uses_heuristic_when_no_api_key(self, monkeypatch):
        monkeypatch.setattr("app.classifier.settings.gemini_api_key", "")
        event = make_event("Thanks for the quick help!")
        classification, source = classify(event)
        assert source == "heuristic"
        assert classification.sentiment == "positive"

    def test_raises_classification_error_when_llm_exhausted(self, monkeypatch):
        monkeypatch.setattr("app.classifier.settings.gemini_api_key", "sk-test")
        monkeypatch.setattr(
            "app.classifier.classify_with_llm",
            MagicMock(side_effect=ValueError("boom")),
        )
        event = make_event("Some message")
        with pytest.raises(ClassificationError):
            classify(event)
