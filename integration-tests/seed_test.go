// Package integrationtests runs the tests against a real Gogs server. The
// package only has test files, so none of its code, e.g., seeding, is built
// into Gogs.
package integrationtests

// Seeding goes through the stores of the application rather than raw SQL, so
// seeded data is in the same state as data created through the server, e.g.,
// hashed passwords and repositories with Git directories, while the plaintext
// secrets are returned to the caller. Repositories get Git hooks that call
// conf.AppPath(), which must be the binary of the server under test.

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/cockroachdb/errors"
	"github.com/pquerna/otp/totp"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
)

// Init loads the configuration of the server under test from the given file and
// connects to its database. The server must have started once to migrate the
// database.
func Init(logger *logx.Logger, configPath string) error {
	err := conf.Init(logger, configPath)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}
	_, err = database.SetEngine(logger, false)
	if err != nil {
		return errors.Wrap(err, "set engine")
	}
	// The README template of auto-initialized repositories is looked up from
	// the loaded list.
	database.LoadRepoConfig(logger)
	return nil
}

// UniqueName returns the prefix followed by random characters, so that data
// seeded by tests running at the same time, or by an earlier run against the
// same database, never conflicts.
func UniqueName(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// User is a seeded user with the password to sign in with.
type User struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	// ActivationCode is the code mailed to an inactive user to activate the
	// account.
	ActivationCode string `json:"activationCode,omitempty"`
}

type CreateUserOptions struct {
	Admin    bool
	Inactive bool
}

// CreateUser creates a user with a unique name.
func CreateUser(ctx context.Context, opts CreateUserOptions) (*User, error) {
	name := UniqueName("user-")
	password := UniqueName("password-")
	u, err := database.Handle.Users().Create(ctx, name, name+"@example.com", database.CreateUserOptions{
		Password:  password,
		Activated: !opts.Inactive,
		Admin:     opts.Admin,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create user")
	}

	user := &User{
		ID:       u.ID,
		Name:     u.Name,
		Email:    u.Email,
		Password: password,
	}
	if opts.Inactive {
		user.ActivationCode = userx.GenerateActivateCode(u.ID, u.Email, u.Name, u.Password, u.Rands)
	}
	return user, nil
}

// CreateAccessToken creates an access token for the user and returns the token
// to authenticate with, which the database only stores the hash of.
func CreateAccessToken(ctx context.Context, userID int64) (string, error) {
	t, err := database.Handle.AccessTokens().Create(ctx, userID, UniqueName("token-"))
	if err != nil {
		return "", errors.Wrap(err, "create access token")
	}
	return t.Sha1, nil
}

// TwoFactor is the two-factor authentication of a seeded user.
type TwoFactor struct {
	// Secret generates the passcodes, e.g., with totp.GenerateCode.
	Secret        string   `json:"secret"`
	RecoveryCodes []string `json:"recoveryCodes"`
}

// EnableTwoFactor enables two-factor authentication for the user.
func EnableTwoFactor(ctx context.Context, userID int64) (*TwoFactor, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      conf.App.BrandName,
		AccountName: UniqueName("account-"),
	})
	if err != nil {
		return nil, errors.Wrap(err, "generate TOTP key")
	}
	err = database.Handle.TwoFactors().Create(ctx, userID, conf.Security.SecretKey, key.Secret())
	if err != nil {
		return nil, errors.Wrap(err, "create two factor")
	}

	codes, err := database.GetRecoveryCodesByUserID(userID)
	if err != nil {
		return nil, errors.Wrap(err, "get recovery codes")
	}
	tf := &TwoFactor{Secret: key.Secret()}
	for _, c := range codes {
		tf.RecoveryCodes = append(tf.RecoveryCodes, c.Code)
	}
	return tf, nil
}

// Repository is a seeded repository.
type Repository struct {
	ID    int64  `json:"id"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

type CreateRepositoryOptions struct {
	Private bool
	// AutoInit adds an initial commit with a README.md.
	AutoInit bool
}

// CreateRepository creates a repository with a unique name owned by the user.
func CreateRepository(ctx context.Context, logger *logx.Logger, ownerID int64, opts CreateRepositoryOptions) (*Repository, error) {
	owner, err := database.Handle.Users().GetByID(ctx, ownerID)
	if err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	readme := ""
	if opts.AutoInit {
		readme = "Default"
	}
	repo, err := database.CreateRepository(ctx, logger, owner, owner, database.CreateRepoOptionsLegacy{
		Name:      UniqueName("repo-"),
		IsPrivate: opts.Private,
		AutoInit:  opts.AutoInit,
		Readme:    readme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create repository")
	}
	return &Repository{ID: repo.ID, Owner: owner.Name, Name: repo.Name}, nil
}
