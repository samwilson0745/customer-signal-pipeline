from unittest.mock import MagicMock

from app import cassandra_client, es_client, redis_client


def test_cassandra_ping_false_when_not_connected(monkeypatch):
    monkeypatch.setattr(cassandra_client, "_session", None)
    assert cassandra_client.ping() is False


def test_cassandra_ping_true_when_session_responds(monkeypatch):
    session = MagicMock()
    monkeypatch.setattr(cassandra_client, "_session", session)
    assert cassandra_client.ping() is True
    session.execute.assert_called_once()


def test_cassandra_ping_false_when_session_raises(monkeypatch):
    session = MagicMock()
    session.execute.side_effect = RuntimeError("boom")
    monkeypatch.setattr(cassandra_client, "_session", session)
    assert cassandra_client.ping() is False


def test_es_ping_false_when_not_connected(monkeypatch):
    monkeypatch.setattr(es_client, "_client", None)
    assert es_client.ping() is False


def test_es_ping_reflects_client_response(monkeypatch):
    client = MagicMock()
    client.ping.return_value = True
    monkeypatch.setattr(es_client, "_client", client)
    assert es_client.ping() is True

    client.ping.side_effect = RuntimeError("boom")
    assert es_client.ping() is False


def test_redis_ping_false_when_not_connected(monkeypatch):
    monkeypatch.setattr(redis_client, "_client", None)
    assert redis_client.ping() is False


def test_redis_ping_reflects_client_response(monkeypatch):
    client = MagicMock()
    client.ping.return_value = True
    monkeypatch.setattr(redis_client, "_client", client)
    assert redis_client.ping() is True

    client.ping.side_effect = RuntimeError("boom")
    assert redis_client.ping() is False
