package concurrency

import (
	"sync"
	"time"
)

// mutexes to queue mass simultaneous requests of same url
var UrlMutexes = make(map[string]*mutexWithTime)
var UrlMutexesLock sync.Mutex

// store access time with mutex to prevent deleting
// mutex that is currently being used
type mutexWithTime struct {
	sync.Mutex
	lastAccess time.Time
}

// creates or retrieves a mutex for a given URL from the urlMutexes map
func GetOrCreateUrlMutex(url string) *mutexWithTime {
	// Lock access to the urlMutexes map
	UrlMutexesLock.Lock()
	defer UrlMutexesLock.Unlock()

	// check if a mutex already exists for the url
	if mutex, ok := UrlMutexes[url]; ok {
		// update the last access time
		mutex.lastAccess = time.Now()
		return mutex
	}

	// if mutex doesn't exist, create a new one and add it to the map
	mutex := &mutexWithTime{lastAccess: time.Now()}
	UrlMutexes[url] = mutex
	return mutex
}

// cleans up the UrlMutexes map by removing mutexes that have not been accessed in the last minute.
func CleanUrlMutexes(now time.Time) {
	UrlMutexesLock.Lock()
	defer UrlMutexesLock.Unlock()

	for url, mutex := range UrlMutexes {
		if now.Sub(mutex.lastAccess) > time.Minute {
			delete(UrlMutexes, url)
		}
	}
}
