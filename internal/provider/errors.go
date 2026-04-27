package provider

import "errors"

// ErrUnsupportedProvider is returned when a provider name is not registered.
func ErrUnsupportedProvider(name string) error {
	return errors.New("unsupported provider: " + name)
}

// ErrStreamClosed is returned when reading from a closed stream.
var ErrStreamClosed = errors.New("stream closed")
