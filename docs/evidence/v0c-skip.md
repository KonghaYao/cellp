# V0c Skipped — Single VIP Baseline

**Date:** 2026-08-27  
**Reason:** Dev and prod RustFS deployments use a single VIP / single-node endpoint. Multi-node conditional-write validation (TP-V0c) does not apply until production runs ≥2 RustFS nodes with clients connecting to different endpoints.  
**Impact:** No change to M1/M2 gates. Re-run V0c when multi-endpoint RustFS topology is deployed.
