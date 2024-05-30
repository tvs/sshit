package sshit

import "fmt"

// Endpoint contains a host path and port used when establishing SSH
// connections.
type Endpoint struct {
	Host string
	Port int
}

// Address returns a string containing the host and port for an endpoint.
func (e Endpoint) Address() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}
