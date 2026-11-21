package platform

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Coordinator interface {
	AllowFixedWindow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
	AcquireSlot(ctx context.Context, scope, holder string, capacity int, ttl time.Duration) (bool, error)
	ReleaseSlot(ctx context.Context, scope, holder string) error
	Ping(ctx context.Context) error
	Close() error
}

func NewCoordinator(redisURL string) (Coordinator, error) {
	if redisURL == "" {
		return NewMemoryCoordinator(), nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	return &RedisCoordinator{client: client}, nil
}

var rateLimitLua = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
if current > tonumber(ARGV[1]) then
  return 0
end
return 1
`)

var slotAcquireLua = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local exp = tonumber(ARGV[2])
local cap = tonumber(ARGV[3])
local holder = ARGV[4]
redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
local existing = redis.call("ZSCORE", key, holder)
if existing then
  redis.call("ZADD", key, exp, holder)
  return 1
end
local count = redis.call("ZCARD", key)
if count >= cap then
  return 0
end
redis.call("ZADD", key, exp, holder)
redis.call("PEXPIRE", key, (exp - now) + 60000)
return 1
`)

type RedisCoordinator struct {
	client *redis.Client
}

func (r *RedisCoordinator) AllowFixedWindow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := fmt.Sprintf("canvas:rate:%s", key)
	res, err := rateLimitLua.Run(ctx, r.client, []string{fullKey}, limit, window.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (r *RedisCoordinator) AcquireSlot(ctx context.Context, scope, holder string, capacity int, ttl time.Duration) (bool, error) {
	fullKey := fmt.Sprintf("canvas:slots:%s", scope)
	now := time.Now().UnixMilli()
	exp := now + ttl.Milliseconds()
	res, err := slotAcquireLua.Run(ctx, r.client, []string{fullKey}, now, exp, capacity, holder).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (r *RedisCoordinator) ReleaseSlot(ctx context.Context, scope, holder string) error {
	fullKey := fmt.Sprintf("canvas:slots:%s", scope)
	return r.client.ZRem(ctx, fullKey, holder).Err()
}

func (r *RedisCoordinator) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisCoordinator) Close() error {
	return r.client.Close()
}

type windowEntry struct {
	count     int
	expiresAt time.Time
}

type MemoryCoordinator struct {
	mu      sync.Mutex
	windows map[string]windowEntry
	slots   map[string]map[string]time.Time
}

func NewMemoryCoordinator() *MemoryCoordinator {
	return &MemoryCoordinator{
		windows: make(map[string]windowEntry),
		slots:   make(map[string]map[string]time.Time),
	}
}

func (m *MemoryCoordinator) AllowFixedWindow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	entry, ok := m.windows[key]
	if !ok || !now.Before(entry.expiresAt) {
		m.windows[key] = windowEntry{count: 1, expiresAt: now.Add(window)}
		return limit >= 1, nil
	}
	if entry.count >= limit {
		return false, nil
	}
	entry.count++
	m.windows[key] = entry
	return true, nil
}

func (m *MemoryCoordinator) AcquireSlot(_ context.Context, scope, holder string, capacity int, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	holders, ok := m.slots[scope]
	if !ok {
		holders = make(map[string]time.Time)
		m.slots[scope] = holders
	}
	for h, exp := range holders {
		if !now.Before(exp) {
			delete(holders, h)
		}
	}
	if _, exists := holders[holder]; exists {
		holders[holder] = now.Add(ttl)
		return true, nil
	}
	if len(holders) >= capacity {
		return false, nil
	}
	holders[holder] = now.Add(ttl)
	return true, nil
}

func (m *MemoryCoordinator) ReleaseSlot(_ context.Context, scope, holder string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if holders, ok := m.slots[scope]; ok {
		delete(holders, holder)
	}
	return nil
}

func (m *MemoryCoordinator) Ping(_ context.Context) error {
	return nil
}

func (m *MemoryCoordinator) Close() error {
	return nil
}
