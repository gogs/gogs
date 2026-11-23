// Package options builds, from the loaded configuration, the values that
// packages take as arguments. It is the only place that reads the whole
// configuration, so every other function receives only what it needs.
package options

import (
	"time"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/ssh"
)

// Links returns the settings that links to users and repositories are made of.
func Links(cfg *conf.Config) database.LinkOptions {
	return database.LinkOptions{
		ExternalURL: cfg.Server.ExternalURL,
		Subpath:     cfg.Server.Subpath,
		SecretKey:   cfg.Security.SecretKey,
		RunUser:     cfg.App.RunUser,
		SSHDomain:   cfg.SSH.Domain,
		SSHPort:     cfg.SSH.Port,
	}
}

// Markup returns the settings that rendering reads.
func Markup(cfg *conf.Config) markup.Options {
	return markup.Options{
		ExternalURL:        cfg.Server.ExternalURL,
		Subpath:            cfg.Server.Subpath,
		SubpathDepth:       cfg.Server.SubpathDepth,
		FileHardLineBreak:  cfg.Markdown.EnableHardLineBreak,
		Smartypants:        cfg.Smartypants,
		MaxDisplayFileSize: cfg.UI.MaxDisplayFileSize,
	}
}

// Email returns the settings that sending mail reads. In hook mode, sending
// blocks until the message is sent.
func Email(cfg *conf.Config, hookMode bool) email.Options {
	return email.Options{
		Email:                  cfg.Email,
		BrandName:              cfg.App.BrandName,
		ExternalURL:            cfg.Server.ExternalURL,
		LoadAssetsFromDisk:     cfg.Server.LoadAssetsFromDisk,
		ActivateCodeLives:      cfg.Auth.ActivateCodeLives,
		ResetPasswordCodeLives: cfg.Auth.ResetPasswordCodeLives,
		Markup:                 Markup(cfg),
		HookMode:               hookMode,
	}
}

// Notify returns the settings of notifying about issues and pull requests.
func Notify(cfg *conf.Config, hookMode bool) database.NotifyOptions {
	return database.NotifyOptions{
		Links:                   Links(cfg),
		RepositoryRoot:          cfg.Repository.Root,
		Markup:                  Markup(cfg),
		EnableEmailNotification: cfg.User.EnableEmailNotification,
		Email:                   Email(cfg, hookMode),
	}
}

// Push returns the settings of recording pushes.
func Push(cfg *conf.Config, hookMode bool) database.PushOptions {
	return database.PushOptions{
		FeedMaxCommitNum: cfg.UI.FeedMaxCommitNum,
		AppDataPath:      cfg.Server.AppDataPath,
		Notify:           Notify(cfg, hookMode),
	}
}

// AuthorizedKeys returns the settings of the authorized_keys file.
func AuthorizedKeys(cfg *conf.Config) database.AuthorizedKeysOptions {
	return database.NewAuthorizedKeysOptions(cfg.SSH, cfg.CustomConf)
}

// RepoCreation returns the settings of creating repositories.
func RepoCreation(cfg *conf.Config) database.RepoCreationOptions {
	return database.RepoCreationOptions{
		Root:             cfg.Repository.Root,
		ForcePrivate:     cfg.Repository.ForcePrivate,
		MaxCreationLimit: cfg.Repository.MaxCreationLimit,
		DefaultBranch:    cfg.Repository.DefaultBranch,
		AppDataPath:      cfg.Server.AppDataPath,
		Links:            Links(cfg),
	}
}

// DeleteRepo returns the settings of deleting repositories.
func DeleteRepo(cfg *conf.Config) database.DeleteRepoOptions {
	return database.DeleteRepoOptions{
		RepositoryRoot: cfg.Repository.Root,
		AttachmentPath: cfg.Attachment.Path,
		AuthorizedKeys: AuthorizedKeys(cfg),
	}
}

// DeleteUser returns the settings of deleting users and organizations.
func DeleteUser(cfg *conf.Config) database.DeleteUserOptions {
	return database.DeleteUserOptions{
		RepositoryRoot:   cfg.Repository.Root,
		AvatarUploadPath: cfg.Picture.AvatarUploadPath,
		AppDataPath:      cfg.Server.AppDataPath,
		AuthorizedKeys:   AuthorizedKeys(cfg),
	}
}

// DelegateHooks returns the settings of the Git hooks of repositories.
func DelegateHooks(cfg *conf.Config) database.DelegateHooksOptions {
	return database.DelegateHooksOptions{
		RepositoryRoot: cfg.Repository.Root,
		ScriptType:     cfg.Repository.ScriptType,
		CustomConf:     cfg.CustomConf,
	}
}

// DeliverHooks returns the settings of delivering webhooks.
func DeliverHooks(cfg *conf.Config) database.DeliverHooksOptions {
	return database.DeliverHooksOptions{
		Webhook:               cfg.Webhook,
		LocalNetworkAllowlist: cfg.Security.LocalNetworkAllowlist,
	}
}

// SyncMirrors returns the settings of syncing mirrors.
func SyncMirrors(cfg *conf.Config) database.SyncMirrorsOptions {
	return database.SyncMirrorsOptions{
		Timeout:               time.Duration(cfg.Git.Timeout.Mirror) * time.Second,
		LocalNetworkAllowlist: cfg.Security.LocalNetworkAllowlist,
		Push:                  Push(cfg, false),
	}
}

// SSHServe returns the settings of serving Git commands over SSH.
func SSHServe(cfg *conf.Config, hookMode bool) ssh.ServeSettings {
	return ssh.ServeSettings{
		Disabled:                 cfg.SSH.Disabled,
		RequireSigninView:        cfg.Auth.RequireSigninView,
		RequireEmailConfirmation: cfg.Auth.RequireEmailConfirmation,
		RepositoryRoot:           cfg.Repository.Root,
		Push:                     Push(cfg, hookMode),
	}
}

// Injected returns the values that HTTP handlers declare as parameters, i.e.,
// the configuration sections and the values built from them, for mapping into
// a flamego.Flame. Each handler declares only the ones it reads.
func Injected(cfg *conf.Config) []any {
	return []any{
		cfg.App, cfg.Server, cfg.SSH, cfg.Repository, cfg.Security, cfg.Email,
		cfg.Auth, cfg.User, cfg.Session, cfg.Attachment, cfg.Picture, cfg.Mirror,
		cfg.LFS, cfg.HTTP, cfg.Release, cfg.Webhook, cfg.Markdown, cfg.Smartypants,
		cfg.Admin, cfg.Cron, cfg.Git, cfg.API, cfg.UI, cfg.Prometheus, cfg.Tracing,
		cfg.I18n,
		Links(cfg), Markup(cfg), Email(cfg, false), Notify(cfg, false), Push(cfg, false),
		AuthorizedKeys(cfg), RepoCreation(cfg), DeleteRepo(cfg), DeleteUser(cfg),
		DelegateHooks(cfg),
	}
}
