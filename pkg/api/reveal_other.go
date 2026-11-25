//go:build !windows

package api

// defaultReveal is nil off Windows: there is no revealer, the route answers 501 and meta says
// capabilities.reveal is false.
var defaultReveal revealer
