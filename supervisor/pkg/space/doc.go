// Package space is the supervisor-owned space layer.
//
// A space is a persistent data namespace identified by the configuration
// persisted by the Space service.
// Storage is pluggable via the Store interface; the product implementation
// calls the Space service through its canonical NATS service functions.
// The supervisor domain type (Space) exposes metadata received from the
// Space-service-owned configuration record.
package space
