// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"reflect"
	"runtime"

	"github.com/urfave/cli"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/setting"
)

var (
	Admin = cli.Command{
		Name:  "admin",
		Usage: "Perform admin operations on command line",
		Description: `Allow using internal logic of Gogs without hacking into the source code
to make automatic initialization process more smoothly`,
		Subcommands: []cli.Command{
			subcmdCreateUser,
			subcmdDeleteInactivateUsers,
			subcmdDeleteRepositoryArchives,
			subcmdDeleteMissingRepositories,
			subcmdGitGcRepos,
			subcmdRewriteAllPublicKeys,
			subcmdSyncRepositoryHooks,
			subcmdReinitMissingRepositories,
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
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdDeleteInactivateUsers = cli.Command{
		Name:  "delete-inactive-users",
		Usage: "Delete all inactive accounts",
		Action: adminDashboardOperation(
			models.DeleteInactivateUsers,
			"All inactivate accounts have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdDeleteRepositoryArchives = cli.Command{
		Name:  "delete-repository-archives",
		Usage: "Delete all repositories archives",
		Action: adminDashboardOperation(
			models.DeleteRepositoryArchives,
			"All repositories archives have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdDeleteMissingRepositories = cli.Command{
		Name:  "delete-missing-repositories",
		Usage: "Delete all repository records that lost Git files",
		Action: adminDashboardOperation(
			models.DeleteMissingRepositories,
			"All repositories archives have been deleted successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdGitGcRepos = cli.Command{
		Name:  "collect-garbage",
		Usage: "Do garbage collection on repositories",
		Action: adminDashboardOperation(
			models.GitGcRepos,
			"All repositories have done garbage collection successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdRewriteAllPublicKeys = cli.Command{
		Name:  "rewrite-public-keys",
		Usage: "Rewrite '.ssh/authorized_keys' file (caution: non-Gogs keys will be lost)",
		Action: adminDashboardOperation(
			models.RewriteAllPublicKeys,
			"All public keys have been rewritten successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdSyncRepositoryHooks = cli.Command{
		Name:  "resync-hooks",
		Usage: "Resync pre-receive, update and post-receive hooks",
		Action: adminDashboardOperation(
			models.SyncRepositoryHooks,
			"All repositories' pre-receive, update and post-receive hooks have been resynced successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}

	subcmdReinitMissingRepositories = cli.Command{
		Name:  "reinit-missing-repositories",
		Usage: "Reinitialize all repository records that lost Git files",
		Action: adminDashboardOperation(
			models.ReinitMissingRepositories,
			"All repository records that lost Git files have been reinitialized successfully",
		),
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}
)

func runCreateUser(c *cli.Context) error {
	if !c.IsSet("name") {
		return fmt.Errorf("Username is not specified")
	} else if !c.IsSet("password") {
		return fmt.Errorf("Password is not specified")
	} else if !c.IsSet("email") {
		return fmt.Errorf("Email is not specified")
	}

	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}

	setting.NewContext()
	models.LoadConfigs()
	models.SetEngine()

	if err := models.CreateUser(&models.User{
		Name:     c.String("name"),
		Email:    c.String("email"),
		Passwd:   c.String("password"),
		IsActive: true,
		IsAdmin:  c.Bool("admin"),
	}); err != nil {
		return fmt.Errorf("CreateUser: %v", err)
	}

	fmt.Printf("New user '%s' has been successfully created!\n", c.String("name"))
	return nil
}

func adminDashboardOperation(operation func() error, successMessage string) func(*cli.Context) error {
	return func(c *cli.Context) error {
		if c.IsSet("config") {
			setting.CustomConf = c.String("config")
		}

		setting.NewContext()
		models.LoadConfigs()
		models.SetEngine()

		if err := operation(); err != nil {
			functionName := runtime.FuncForPC(reflect.ValueOf(operation).Pointer()).Name()
			return fmt.Errorf("%s: %v", functionName, err)
		}

		fmt.Printf("%s\n", successMessage)
		return nil
	}
}
