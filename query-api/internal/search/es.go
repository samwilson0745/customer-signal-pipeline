// Package search wraps Elasticsearch full-text + filtered search behind a
// small typed interface used by the /search handler.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	elasticsearch "github.com/elastic/go-elasticsearch/v8"
)

type Query struct {
	Text      string
	Brand     string
	Sentiment string
	Urgency   string
}

type Hit struct {
	EventID   string `json:"event_id"`
	Channel   string `json:"channel"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	Brand     string `json:"brand"`
	Sentiment string `json:"sentiment"`
	Urgency   string `json:"urgency"`
	Topic     string `json:"topic"`
	CreatedAt string `json:"created_at"`
}

type Client struct {
	es    *elasticsearch.Client
	index string
}

func NewClient(url, index string) (*Client, error) {
	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{url}})
	if err != nil {
		return nil, err
	}
	return &Client{es: es, index: index}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	res, err := c.es.Ping(c.es.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch ping returned status %s", res.Status())
	}
	return nil
}

func buildQuery(q Query) map[string]interface{} {
	must := []map[string]interface{}{}
	if q.Text != "" {
		must = append(must, map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  q.Text,
				"fields": []string{"text", "author", "topic"},
			},
		})
	} else {
		must = append(must, map[string]interface{}{"match_all": map[string]interface{}{}})
	}

	filter := []map[string]interface{}{}
	addTermFilter := func(field, value string) {
		if value != "" {
			filter = append(filter, map[string]interface{}{"term": map[string]interface{}{field: value}})
		}
	}
	addTermFilter("brand", q.Brand)
	addTermFilter("sentiment", q.Sentiment)
	addTermFilter("urgency", q.Urgency)

	return map[string]interface{}{
		"size": 50,
		"sort": []map[string]interface{}{{"created_at": map[string]interface{}{"order": "desc"}}},
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filter,
			},
		},
	}
}

// Search executes the query against Elasticsearch and returns matching
// documents. The caller (handler) is responsible for wrapping this call in
// a circuit breaker.
func (c *Client) Search(ctx context.Context, q Query) ([]Hit, error) {
	body := buildQuery(q)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.index),
		c.es.Search.WithBody(bytes.NewReader(payload)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search returned status %s", res.Status())
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source Hit `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	hits := make([]Hit, 0, len(parsed.Hits.Hits))
	for _, h := range parsed.Hits.Hits {
		hits = append(hits, h.Source)
	}
	return hits, nil
}
