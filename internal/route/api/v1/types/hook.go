package types

import "time"

type RepositoryHook struct {
	ID      int64             `json:"id"`
	Type    string            `json:"type"`
	URL     string            `json:"-"`
	Config  map[string]string `json:"config"`
	Events  []string          `json:"events"`
	Active  bool              `json:"active"`
	Updated time.Time         `json:"updated_at"`
	Created time.Time         `json:"created_at"`
}

type RepositoryDeployKey struct {
	ID       int64     `json:"id"`
	Key      string    `json:"key"`
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created_at"`
	ReadOnly bool      `json:"read_only"`
}
