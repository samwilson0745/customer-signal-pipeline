// Package stats reads the live sentiment/urgency counters the triage
// service maintains in Redis (keys "stats:{brand}:sentiment:{value}" and
// "stats:{brand}:urgency:{value}"), so /stats never has to touch
// Elasticsearch or Cassandra.
package stats

import (
	"context"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type Breakdown struct {
	Brand     string         `json:"brand"`
	Total     int64          `json:"total"`
	Sentiment map[string]int64 `json:"sentiment"`
	Urgency   map[string]int64 `json:"urgency"`
}

type Client struct {
	rdb *redis.Client
}

func NewClient(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) scanCounters(ctx context.Context, pattern string) (map[string]int64, error) {
	result := map[string]int64{}
	iter := c.rdb.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return result, nil
	}

	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		parts := strings.Split(key, ":")
		label := parts[len(parts)-1]
		if values[i] == nil {
			continue
		}
		n, _ := strconv.ParseInt(values[i].(string), 10, 64)
		result[label] = n
	}
	return result, nil
}

// GetBreakdown returns the sentiment/urgency breakdown for a brand. The
// caller (handler) wraps this in a circuit breaker.
func (c *Client) GetBreakdown(ctx context.Context, brand string) (*Breakdown, error) {
	sentiment, err := c.scanCounters(ctx, "stats:"+brand+":sentiment:*")
	if err != nil {
		return nil, err
	}
	urgency, err := c.scanCounters(ctx, "stats:"+brand+":urgency:*")
	if err != nil {
		return nil, err
	}
	totalStr, err := c.rdb.Get(ctx, "stats:"+brand+":total").Result()
	var total int64
	if err == nil {
		total, _ = strconv.ParseInt(totalStr, 10, 64)
	} else if err != redis.Nil {
		return nil, err
	}

	return &Breakdown{Brand: brand, Total: total, Sentiment: sentiment, Urgency: urgency}, nil
}
