// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package workers

// Work represents a background work interface of any kind.
type Work interface {
	Do() error
}
