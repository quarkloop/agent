// Package plugin defines shared plugin interfaces, types, manifest parsing, and loader.
//
// It provides the Plugin interface and type-specific plugin contracts along with
// manifest parsing (manifest.yaml) and a loader that supports both lib mode
// (.so via plugin.Open) and api mode (HTTP server processes).
package plugin
