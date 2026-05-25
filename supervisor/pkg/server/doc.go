// Package server composes the supervisor NATS-native control plane.
//
// The package resolves supervisor-owned plugin catalogs, delegates physical
// space persistence to the Space service, and exposes high-level operations
// through the NATS API host.
package server
