package cache

import (
	"container/list"
	"errors"
	"time"
)

type EvictionPolicy interface {
	SelectForEviction(data map[string]*CacheEntry) (string, error)
}

type LRUPolicy struct{}

func NewLRUPolicy() EvictionPolicy {
	return &LRUPolicy{}
}

func (lru *LRUPolicy) SelectForEviction(data map[string]*CacheEntry) (string, error) {
	var oldestKey string
	var oldestTime time.Time

	for key, e := range data {
		if oldestKey == "" || e.AccessedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = e.AccessedAt
		}
	}

	if oldestKey == "" {
		return "", errors.New("no entries to evict")
	}

	return oldestKey, nil
}

type LFUPolicy struct{}

func NewLFUPolicy() EvictionPolicy {
	return &LFUPolicy{}
}

func (lfu *LFUPolicy) SelectForEviction(data map[string]*CacheEntry) (string, error) {
	var leastUsed string
	var minCount = ^uint64(0)

	for key, e := range data {
		if e.AccessCount < minCount {
			leastUsed = key
			minCount = e.AccessCount
		}
	}

	if leastUsed == "" {
		return "", errors.New("no entries to evict")
	}

	return leastUsed, nil
}

type FIFOPolicy struct{}

func NewFIFOPolicy() EvictionPolicy {
	return &FIFOPolicy{}
}

func (fifo *FIFOPolicy) SelectForEviction(data map[string]*CacheEntry) (string, error) {
	var oldestKey string
	var oldestTime time.Time

	for key, e := range data {
		if oldestKey == "" || e.Value.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = e.Value.CreatedAt
		}
	}

	if oldestKey == "" {
		return "", errors.New("no entries to evict")
	}

	return oldestKey, nil
}

type SizeBasedPolicy struct {
	list *list.List
}

func NewSizeBasedPolicy() EvictionPolicy {
	return &SizeBasedPolicy{
		list: list.New(),
	}
}

func (sb *SizeBasedPolicy) SelectForEviction(data map[string]*CacheEntry) (string, error) {
	var largestKey string
	var largestSize int64

	for key, e := range data {
		if largestKey == "" || e.Size > largestSize {
			largestKey = key
			largestSize = e.Size
		}
	}

	if largestKey == "" {
		return "", errors.New("no entries to evict")
	}

	return largestKey, nil
}
