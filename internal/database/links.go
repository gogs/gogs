package database

import (
	"gogs.io/gogs/internal/repox"
)

// LinkOptions are the settings that links to users and repositories are made of.
type LinkOptions struct {
	// ExternalURL is the external URL of the server, ending with "/".
	ExternalURL string
	// Subpath is the path of ExternalURL without the trailing "/".
	Subpath string
	// SecretKey hashes the keys of the URLs of generated avatars.
	SecretKey string
	// RunUser, SSHDomain and SSHPort make the SSH clone URLs.
	RunUser   string
	SSHDomain string
	SSHPort   int
}

// CloneLinkOptions returns the options that clone links are made with.
func (o LinkOptions) CloneLinkOptions() repox.CloneLinkOptions {
	return repox.CloneLinkOptions{
		ExternalURL: o.ExternalURL,
		RunUser:     o.RunUser,
		SSHDomain:   o.SSHDomain,
		SSHPort:     o.SSHPort,
	}
}
