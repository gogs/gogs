package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/cmd/gogs/internal/options"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

var (
	adminCommand = cli.Command{
		Name:  "admin",
		Usage: "Perform admin operations on command line",
		Description: `Allow using internal logic of Gogs without hacking into the source code
to make automatic initialization process more smoothly`,
		Commands: []*cli.Command{
			&subcmdCreateUser,
			&subcmdDeleteInactivateUsers,
			&subcmdDeleteRepositoryArchives,
			&subcmdDeleteMissingRepositories,
			&subcmdGitGcRepos,
			&subcmdRewriteAuthorizedKeys,
			&subcmdSyncRepositoryHooks,
			&subcmdReinitMissingRepositories,
		},
	}

	subcmdCreateUser = cli.Command{
		Name:   "create-user",
		Usage:  "Create a new user in database",
		Action: runCreateUser,
		Flags: []cli.Flag{
			stringFlag("name", "", "Username"),
			stringFlag("password", "", "User password"),
			stringFlag("email", "", "User email address"),
			boolFlag("admin", "User is an admin"),
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdDeleteInactivateUsers = cli.Command{
		Name:  "delete-inactive-users",
		Usage: "Delete all inactive accounts",
		Action: adminDashboardOperation(
			func(_ *logx.Logger, cfg *conf.Config) error {
				return database.Handle.Users().DeleteInactivated(options.DeleteUser(cfg))
			},
			"All inactivated accounts have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdDeleteRepositoryArchives = cli.Command{
		Name:  "delete-repository-archives",
		Usage: "Delete all repositories archives",
		Action: adminDashboardOperation(
			func(_ *logx.Logger, cfg *conf.Config) error {
				return database.DeleteRepositoryArchives(cfg.Repository.Root)
			},
			"All repositories archives have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdDeleteMissingRepositories = cli.Command{
		Name:  "delete-missing-repositories",
		Usage: "Delete all repository records that lost Git files",
		Action: adminDashboardOperation(
			func(logger *logx.Logger, cfg *conf.Config) error {
				return database.DeleteMissingRepositories(logger, options.DeleteRepo(cfg))
			},
			"All repositories archives have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdGitGcRepos = cli.Command{
		Name:  "collect-garbage",
		Usage: "Do garbage collection on repositories",
		Action: adminDashboardOperation(
			func(_ *logx.Logger, cfg *conf.Config) error {
				return database.GitGcRepos(cfg.Repository.Root, time.Duration(cfg.Git.Timeout.GC)*time.Second, cfg.Git.GCArgs)
			},
			"All repositories have done garbage collection successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdRewriteAuthorizedKeys = cli.Command{
		Name:  "rewrite-authorized-keys",
		Usage: "Rewrite '.ssh/authorized_keys' file (caution: non-Gogs keys will be lost)",
		Action: adminDashboardOperation(
			func(_ *logx.Logger, cfg *conf.Config) error {
				return database.Handle.PublicKeys().RewriteAuthorizedKeys(options.AuthorizedKeys(cfg))
			},
			"All public keys have been rewritten successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdSyncRepositoryHooks = cli.Command{
		Name:  "resync-hooks",
		Usage: "Resync pre-receive, update and post-receive hooks",
		Action: adminDashboardOperation(
			func(_ *logx.Logger, cfg *conf.Config) error {
				return database.SyncRepositoryHooks(options.DelegateHooks(cfg))
			},
			"All repositories' pre-receive, update and post-receive hooks have been resynced successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}

	subcmdReinitMissingRepositories = cli.Command{
		Name:  "reinit-missing-repositories",
		Usage: "Reinitialize all repository records that lost Git files",
		Action: adminDashboardOperation(
			func(logger *logx.Logger, cfg *conf.Config) error {
				return database.ReinitMissingRepositories(logger, cfg.Repository.Root)
			},
			"All repository records that lost Git files have been reinitialized successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}
)

func runCreateUser(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger(os.Stdout)
	if !cmd.IsSet("name") {
		return errors.New("Username is not specified")
	} else if !cmd.IsSet("password") {
		return errors.New("Password is not specified")
	} else if !cmd.IsSet("email") {
		return errors.New("Email is not specified")
	}

	cfg, err := conf.Init(logger, configFromLineage(cmd), false)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}
	if _, err = conf.InitLogging(logger, cfg, true); err != nil {
		return errors.Wrap(err, "init logging")
	}

	if _, err = database.SetEngine(logger.Scoped("db"), cfg.Database, false); err != nil {
		return errors.Wrap(err, "set engine")
	}

	user, err := database.Handle.Users().Create(
		ctx,
		cmd.String("name"),
		cmd.String("email"),
		database.CreateUserOptions{
			Password:  cmd.String("password"),
			Activated: true,
			Admin:     cmd.Bool("admin"),
		},
	)
	if err != nil {
		return errors.Wrap(err, "create user")
	}

	fmt.Printf("New user %q has been successfully created!\n", user.Name)
	return nil
}

func adminDashboardOperation(operation func(*logx.Logger, *conf.Config) error, successMessage string) func(context.Context, *cli.Command) error {
	return func(_ context.Context, cmd *cli.Command) error {
		logger := conf.NewConsoleLogger(os.Stdout)
		cfg, err := conf.Init(logger, configFromLineage(cmd), false)
		if err != nil {
			return errors.Wrap(err, "init configuration")
		}
		if _, err = conf.InitLogging(logger, cfg, true); err != nil {
			return errors.Wrap(err, "init logging")
		}

		if _, err = database.SetEngine(logger.Scoped("db"), cfg.Database, false); err != nil {
			return errors.Wrap(err, "set engine")
		}

		if err := operation(logger, cfg); err != nil {
			return errors.Wrap(err, cmd.Name)
		}

		fmt.Printf("%s\n", successMessage)
		return nil
	}
}
