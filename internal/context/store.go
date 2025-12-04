package context

import (
	"context"

	"gogs.io/gogs/internal/database"
)

// Store is the data layer carrier for context middleware. This interface is
// meant to abstract away and limit the exposure of the underlying data layer to
// the handler through a thin-wrapper.
type Store interface {
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx context.Context, id int64) error

	// GetUserByID returns the user with given ID. It returns
	// database.ErrUserNotExist when not found.
	GetUserByID(ctx context.Context, id int64) (*database.User, error)
	// GetUserByUsername returns the user with given username. It returns
	// database.ErrUserNotExist when not found.
	GetUserByUsername(ctx context.Context, username string) (*database.User, error)
	// CreateUser creates a new user and persists to database. It returns
	// database.ErrNameNotAllowed if the given name or pattern of the name is not
	// allowed as a username, or database.ErrUserAlreadyExist when a user with same
	// name already exists, or database.ErrEmailAlreadyUsed if the email has been
	// verified by another user.
	CreateUser(ctx context.Context, username, email string, opts database.CreateUserOptions) (*database.User, error)
	// AuthenticateUser validates username and password via given login source ID.
	// It returns database.ErrUserNotExist when the user was not found.
	//
	// When the "loginSourceID" is negative, it aborts the process and returns
	// database.ErrUserNotExist if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns
	// database.ErrLoginSourceMismatch if the user has different login source ID
	// than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	AuthenticateUser(ctx context.Context, login, password string, loginSourceID int64) (*database.User, error)
}

type store struct{}

// NewStore returns a new Store using the global database handle.
func NewStore() Store {
	return &store{}
}

func (*store) GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().GetBySHA1(ctx, sha1)
}

func (*store) TouchAccessTokenByID(ctx context.Context, id int64) error {
	return database.Handle.AccessTokens().Touch(ctx, id)
}

func (*store) GetUserByID(ctx context.Context, id int64) (*database.User, error) {
	return database.Handle.Users().GetByID(ctx, id)
}

func (*store) GetUserByUsername(ctx context.Context, username string) (*database.User, error) {
	return database.Handle.Users().GetByUsername(ctx, username)
}

func (*store) CreateUser(ctx context.Context, username, email string, opts database.CreateUserOptions) (*database.User, error) {
	return database.Handle.Users().Create(ctx, username, email, opts)
}

func (*store) AuthenticateUser(ctx context.Context, login, password string, loginSourceID int64) (*database.User, error) {
	return database.Handle.Users().Authenticate(ctx, login, password, loginSourceID)
}
