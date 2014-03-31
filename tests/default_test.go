package test

import (
	"net/http"
	"testing"
)

func TestMain(t *testing.T) {
	r, err := http.Get("http://localhost:3000/")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Error(r.StatusCode)
	}
}
