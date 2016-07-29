// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"sync"
)

// workingPool represents a pool of working status which makes sure
// that only one instance of same task is performing at a time.
// However, different type of tasks can performing at the same time.
type workingPool struct {
	lock  sync.Mutex
	pool  map[string]*sync.Mutex
	count map[string]int
}

// CheckIn checks in a task and waits if others are running.
func (p *workingPool) CheckIn(name string) {
	p.lock.Lock()

	lock, has := p.pool[name]
	if !has {
		lock = &sync.Mutex{}
		p.pool[name] = lock
	}
	p.count[name]++

	p.lock.Unlock()
	lock.Lock()
}

// CheckOut checks out a task to let other tasks run.
func (p *workingPool) CheckOut(name string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool[name].Unlock()
	if p.count[name] == 1 {
		delete(p.pool, name)
		delete(p.count, name)
	} else {
		p.count[name]--
	}
}
