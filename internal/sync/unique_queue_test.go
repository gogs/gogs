package sync

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniqueQueueAddConcurrentDuplicate(t *testing.T) {
	q := NewUniqueQueue(100)
	const goroutines = 50

	q.table.Lock()

	var ready sync.WaitGroup
	ready.Add(goroutines)
	var done sync.WaitGroup
	done.Add(goroutines)
	for range goroutines {
		go func() {
			ready.Done()
			q.Add("same")
			done.Done()
		}()
	}

	ready.Wait()
	q.table.Unlock()
	done.Wait()

	assert.Len(t, q.queue, 1)
}
