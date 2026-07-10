"""LLM-backed message classifier with a deterministic heuristic fallback.

The heuristic path lets the whole pipeline run end-to-end without a
GEMINI_API_KEY (handy for local demos / CI); the LLM path is the "real"
classifier used whenever a key is configured.
"""
import json
import logging
import re
from typing import Optional

from tenacity import retry, retry_if_exception_type, stop_after_attempt, wait_exponential

from app.config import settings
from app.models import Classification, RawEvent

logger = logging.getLogger("triage.classifier")

VALID_SENTIMENTS = {"positive", "neutral", "negative"}
VALID_URGENCIES = {"low", "medium", "high", "critical"}

SENTIMENT_SYNONYMS = {
    "pos": "positive",
    "good": "positive",
    "happy": "positive",
    "neg": "negative",
    "bad": "negative",
    "angry": "negative",
    "upset": "negative",
    "neu": "neutral",
    "mixed": "neutral",
}

URGENCY_SYNONYMS = {
    "none": "low",
    "normal": "medium",
    "urgent": "high",
    "emergency": "critical",
    "blocker": "critical",
}


class ClassificationError(Exception):
    """Raised when a message could not be classified after all retries."""


SYSTEM_PROMPT = (
    "You are a customer support triage assistant. Classify the customer message "
    "below and respond with ONLY a compact JSON object, no prose, in exactly this "
    'shape: {"sentiment": "positive|neutral|negative", '
    '"urgency": "low|medium|high|critical", "topic": "short_snake_case_tag"}.\n'
    "sentiment reflects the customer's tone. urgency reflects how quickly a human "
    "should respond (critical = outage/security/payment failure affecting the "
    "customer right now). topic is a short free-text tag such as billing, outage, "
    "praise, feature_request, account_access, bug_report, or general."
)


def build_prompt(event: RawEvent) -> str:
    return (
        f"{SYSTEM_PROMPT}\n\n"
        f"channel: {event.channel}\n"
        f"message: {event.text}"
    )


_JSON_BLOCK_RE = re.compile(r"\{.*\}", re.DOTALL)


def parse_classification(raw_text: str) -> Classification:
    """Parse (and normalize) a raw LLM completion into a Classification.

    Tolerant of minor deviations from the requested schema: markdown code
    fences, extra prose around the JSON blob, synonym values, wrong casing.
    Raises ValueError if the text cannot be turned into a valid Classification.
    """
    if not raw_text or not raw_text.strip():
        raise ValueError("empty completion from LLM")

    match = _JSON_BLOCK_RE.search(raw_text)
    candidate = match.group(0) if match else raw_text
    try:
        data = json.loads(candidate)
    except json.JSONDecodeError as exc:
        raise ValueError(f"could not parse JSON from completion: {raw_text!r}") from exc

    if not isinstance(data, dict):
        raise ValueError(f"expected a JSON object, got: {type(data)}")

    sentiment = str(data.get("sentiment", "")).strip().lower()
    urgency = str(data.get("urgency", "")).strip().lower()
    topic = str(data.get("topic", "")).strip().lower().replace(" ", "_") or "general"

    sentiment = SENTIMENT_SYNONYMS.get(sentiment, sentiment)
    urgency = URGENCY_SYNONYMS.get(urgency, urgency)

    if sentiment not in VALID_SENTIMENTS:
        raise ValueError(f"invalid sentiment value: {data.get('sentiment')!r}")
    if urgency not in VALID_URGENCIES:
        raise ValueError(f"invalid urgency value: {data.get('urgency')!r}")

    return Classification(sentiment=sentiment, urgency=urgency, topic=topic[:64])


_llm_client: Optional[object] = None


def _get_llm_client():
    global _llm_client
    if _llm_client is None:
        from langchain_google_genai import ChatGoogleGenerativeAI

        _llm_client = ChatGoogleGenerativeAI(
            model=settings.gemini_model,
            google_api_key=settings.gemini_api_key,
            temperature=0,
            timeout=15,
        )
    return _llm_client


@retry(
    reraise=True,
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=0.5, min=0.5, max=4),
    retry=retry_if_exception_type((ValueError, ConnectionError, TimeoutError)),
)
def classify_with_llm(event: RawEvent, llm=None) -> Classification:
    llm = llm or _get_llm_client()
    prompt = build_prompt(event)
    response = llm.invoke(prompt)
    content = getattr(response, "content", response)
    return parse_classification(str(content))


NEGATIVE_WORDS = {"angry", "furious", "ridiculous", "disappointed", "frustrating", "unusable", "annoying", "worst", "hate", "disaster", "unauthorized"}
POSITIVE_WORDS = {"thanks", "thank", "great", "love", "amazing", "seamless", "appreciate", "nice", "shoutout"}
CRITICAL_WORDS = {"urgent", "asap", "critical", "down", "outage", "security", "unauthorized", "immediately", "fix this now"}
HIGH_WORDS = {"frustrating", "refund", "crash", "crashes", "wiped", "disaster"}

TOPIC_KEYWORDS = {
    "billing": {"charge", "charged", "invoice", "billing", "refund", "payment"},
    "outage": {"down", "outage", "500", "crash", "crashes", "crashed"},
    "praise": set(POSITIVE_WORDS),
    "feature_request": {"feature", "roadmap", "dark mode", "export"},
    "account_access": {"login", "log in", "sso", "unauthorized", "access", "logging me out"},
    "bug_report": {"bug", "crash", "error", "errors", "broken"},
}


def classify_heuristic(event: RawEvent) -> Classification:
    text = event.text.lower()

    sentiment = "neutral"
    if any(word in text for word in NEGATIVE_WORDS):
        sentiment = "negative"
    elif any(word in text for word in POSITIVE_WORDS):
        sentiment = "positive"

    urgency = "low"
    if any(word in text for word in CRITICAL_WORDS):
        urgency = "critical"
    elif any(word in text for word in HIGH_WORDS):
        urgency = "high"
    elif sentiment == "negative":
        urgency = "medium"

    topic = "general"
    for candidate_topic, keywords in TOPIC_KEYWORDS.items():
        if any(keyword in text for keyword in keywords):
            topic = candidate_topic
            break

    return Classification(sentiment=sentiment, urgency=urgency, topic=topic)


def classify(event: RawEvent) -> tuple[Classification, str]:
    """Classify an event, returning (classification, source).

    source is "llm" or "heuristic". Falls back to the heuristic classifier
    when no API key is configured, or when the LLM path exhausts its retries
    -- the caller is responsible for deciding whether a heuristic fallback is
    acceptable or whether the event should be dead-lettered instead.
    """
    if not settings.gemini_api_key:
        return classify_heuristic(event), "heuristic"

    try:
        return classify_with_llm(event), "llm"
    except Exception as exc:  # noqa: BLE001 - deliberately broad, we re-raise as our own type
        logger.error("llm classification failed after retries: %s", exc)
        raise ClassificationError(str(exc)) from exc
