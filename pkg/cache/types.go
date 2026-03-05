package cache

import (
	"time"
)

type CacheEntry struct {
	Value       Value
	Size        int64
	AccessedAt  time.Time
	AccessCount uint64
}
