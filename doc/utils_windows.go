package doc

import (
	"errors"
	"os"
)

func HomeDir() (string, error) {
	dir := os.Getenv("userprofile")
	if dir == "" {
		return "", errors.New()
	}

	return dir, nil
}
