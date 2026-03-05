package inventory

import (
	"time"
)

type CachedInventory struct {
	Host       string    `json:"host"`
	Version    string    `json:"version"`
	CachedAt   time.Time `json:"cached_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

type VersionRecord struct {
	Version    string    `json:"version"`
	FirstSeen  time.Time `json:"first_seen"`
	LastUsed   time.Time `json:"last_used"`
	Compatible bool      `json:"compatible"`
}
