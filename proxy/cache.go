package proxy

import (
	"net/http"
	"time"
)

type CacheMetadata struct {
	Header    http.Header
	Body      []byte
	SavedTime time.Time
}

type LocalCache struct {
	Mem map[string]CacheMetadata
}

func CreateLocalCache() *LocalCache {
	return &LocalCache{
		Mem: make(map[string]CacheMetadata),
	}
}

func (cache *LocalCache) CacheGet(pageURL string) *CacheMetadata {
	cacheData := cache.Mem[pageURL]
	if cacheData.SavedTime.IsZero() || time.Now().After(cacheData.SavedTime) {
		delete(cache.Mem, pageURL)
		return nil
	}
	return &cacheData
}

func (cache *LocalCache) CacheSet(pageURL string, Res HTTPResponse, secondsToStore int) int {
	storeDuration := time.Duration(secondsToStore)
	cache.Mem[pageURL] = CacheMetadata{
		Header:    Res.Header,
		Body:      Res.Body,
		SavedTime: time.Now().Add(storeDuration * time.Second),
	}
	return 1
}