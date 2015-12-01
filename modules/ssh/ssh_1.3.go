// +build !go1.4

package ssh

func Listen(port int) {
	panic("Gogs requires Go 1.4 for starting a SSH server")
}
