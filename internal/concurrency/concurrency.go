package concurrency

import "sync"

// mutexes to queue mass simultaneous requests of same url
var UrlMutexes = make(map[string]*sync.Mutex)
var UrlMutexesLock sync.Mutex

// Creates or retrieves a mutex for a given URL from the urlMutexes map.
//
// It takes a URL as a parameter and returns a pointer to a sync.Mutex.
func GetOrCreateUrlMutex(url string) *sync.Mutex {
	// Lock access to the urlMutexes map
	UrlMutexesLock.Lock()
	defer UrlMutexesLock.Unlock()

	// Check if a mutex already exists for the url
	if mutex, ok := UrlMutexes[url]; ok {
		return mutex
	}

	// If mutex doesn't exist, create a new one and add it to the map
	mutex := &sync.Mutex{}
	UrlMutexes[url] = mutex
	return mutex
}
