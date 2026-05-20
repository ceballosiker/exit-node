// Package pfsense defines the pfSense client interface and shared types.
// The concrete community pfsense-api impl lands in Plan 2.
package pfsense

// Gateway is the canonical pfSense gateway record.
type Gateway struct {
	Name string
	IP   string
}
