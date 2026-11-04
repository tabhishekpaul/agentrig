//go:build !windows

package library

import "errors"

func junction(at, real string) error { return errors.New("junctions are Windows only") }
