package errors

import "fmt"

type AccessTokenNameAlreadyExist struct {
	Name string
}

func IsAccessTokenNameAlreadyExist(err error) bool {
	_, ok := err.(AccessTokenNameAlreadyExist)
	return ok
}

func (err AccessTokenNameAlreadyExist) Error() string {
	return fmt.Sprintf("access token already exist [name: %s]", err.Name)
}
