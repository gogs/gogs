package git

import (
	"context"
	"io"
	"os"
	osexec "os/exec"
)

// UpdateServerInfo updates the auxiliary info file on the server side for the
// repository in given path.
func UpdateServerInfo(ctx context.Context, path string) error {
	args := []string{"update-server-info", "--end-of-options"}
	_, err := exec(ctx, path, args, nil)
	return err
}

// Service is a Git service that serves fetches and pushes.
type Service string

// A list of services that clients can request.
const (
	ServiceUploadPack    Service = "upload-pack"
	ServiceUploadArchive Service = "upload-archive"
	ServiceReceivePack   Service = "receive-pack"
)

// ServiceOptions contains optional arguments for running a service.
//
// Docs: https://git-scm.com/docs/git-upload-pack
type ServiceOptions struct {
	// Indicates whether to quit after a single request/response exchange, as the
	// smart HTTP protocol requires.
	StatelessRPC bool
	// Indicates whether to only advertise references and exit, as the "info/refs"
	// endpoint of the smart HTTP protocol requires.
	AdvertiseRefs bool
	// The streams connected to the client.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// The additional environment variables to run the command with.
	Envs []string
}

// RunService runs the service for the repository in given path, e.g., "git
// upload-pack <path>".
func RunService(ctx context.Context, service Service, path string, opts ...ServiceOptions) error {
	var opt ServiceOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{string(service)}
	if opt.StatelessRPC {
		args = append(args, "--stateless-rpc")
	}
	if opt.AdvertiseRefs {
		args = append(args, "--advertise-refs")
	}
	args = append(args, path)

	cmd := osexec.CommandContext(ctx, "git", args...)
	if len(opt.Envs) > 0 {
		cmd.Env = append(os.Environ(), opt.Envs...)
	}
	cmd.Stdin = opt.Stdin
	cmd.Stdout = opt.Stdout
	cmd.Stderr = opt.Stderr
	return cmd.Run()
}
