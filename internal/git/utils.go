package git

import (
	"os"
	"sync"
)

// objectCache provides thread-safe cache operations.
type objectCache struct {
	lock  sync.RWMutex
	cache map[string]interface{}
}

func newObjectCache() *objectCache {
	return &objectCache{
		cache: make(map[string]interface{}),
	}
}

func (oc *objectCache) Set(id string, obj interface{}) {
	oc.lock.Lock()
	defer oc.lock.Unlock()

	oc.cache[id] = obj
}

func (oc *objectCache) Get(id string) (interface{}, bool) {
	oc.lock.RLock()
	defer oc.lock.RUnlock()

	obj, has := oc.cache[id]
	return obj, has
}

// isDir returns true if given path is a directory, or returns false when it's a
// file or does not exist.
func isDir(dir string) bool {
	f, e := os.Stat(dir)
	if e != nil {
		return false
	}
	return f.IsDir()
}

// isFile returns true if given path is a file, or returns false when it's a
// directory or does not exist.
func isFile(filePath string) bool {
	f, e := os.Stat(filePath)
	if e != nil {
		return false
	}
	return !f.IsDir()
}
