package concurrency

import (
	"testing"
	"time"
)

func TestGetOrCreateUrlMutex(t *testing.T) {
	// Test case 1: Creating a new mutex
	url := "https://example.com"
	mutex := GetOrCreateUrlMutex(url)
	if mutex == nil {
		t.Errorf("Expected a mutex, got nil")
	}
	// Test case 2: Retrieving an existing mutex
	mutex2 := GetOrCreateUrlMutex(url)
	if mutex2 != mutex {
		t.Errorf("Expected to retrieve the same mutex, but retrieved a different one")
	}
}

func TestCleanUrlMutexes(t *testing.T) {
	// Set up test data
	url1 := "https://example1.com"
	url2 := "https://example2.com"

	// this should get cleaned up (over 1 min)
	GetOrCreateUrlMutex(url1)

	// advance time by one minute and sleep for 1 second
	now := time.Now().Add(time.Minute)
	time.Sleep(time.Second * 1)

	// this should still exist (within 1 min)
	GetOrCreateUrlMutex(url2)

	// should remove mutexes that haven't been accessed in the last minute
	CleanUrlMutexes(now)

	if _, ok := UrlMutexes[url1]; ok {
		t.Errorf("Expected mutex for url1 to be cleaned up, but it still exists")
	}
	if _, ok := UrlMutexes[url2]; !ok {
		t.Errorf("Expected mutex for url2 to still exist, but it was cleaned up")
	}

	// advance again by one minute
	now = now.Add(time.Minute)
	CleanUrlMutexes(now)
	// mutex2 should be cleaned up
	if _, ok := UrlMutexes[url2]; ok {
		t.Errorf("Expected mutex for url2 to be cleaned up, but it still exists")
	}
}

func BenchmarkGetOrCreateUrlMutex(b *testing.B) {
	url := "https://example.com"
	for i := 0; i < b.N; i++ {
		GetOrCreateUrlMutex(url)
	}
}
