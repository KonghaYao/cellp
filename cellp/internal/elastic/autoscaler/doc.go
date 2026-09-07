// Package autoscaler implements the AD-15 desired-replica control loop (WP-SCALE).
// cellpd always runs this loop; CELLP_ELASTIC_RUNTIME=off is rejected at config load.
package autoscaler
