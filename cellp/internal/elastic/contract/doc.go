// Package contract defines shared SURGE (AD-15) logic types and validation.
//
// WP-CONTRACT is the sole owner of this package. Other components must import
// and must not redefine these semantics. Elastic runtime is always on for cellpd;
// explicit CELLP_ELASTIC_RUNTIME=off fails closed at startup.
package contract
