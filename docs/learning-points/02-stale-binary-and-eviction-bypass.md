# Incident Post-Mortem 02: Stale Binary Execution & Eviction Bypass

## Overview
After updating `main.go` to implement safe workload migration via the Kubernetes `policy/v1` Eviction API, a test `ReimageRequest` was submitted against `kubebuild-lab-worker`. Although the node was marked `Unschedulable`, the active `sample-workload` pods never migrated to `kubebuild-lab-worker2`, and their pod age remained unchanged at 17 minutes.

---

## Observed Behavior vs. Expected Behavior

### Observed Operator Output:
```text
[drain-test] Stage: config_gen -> Validating specifications
[drain-test] Stage: dhcp_add -> Cordoning node 'kubebuild-lab-worker' and triggering Redfish Reset
>> Node kubebuild-lab-worker marked Unschedulable (Cordoned)
>> Simulated Redfish BMC chassis power-cycle sent (204 OK)
[drain-test] Stage: imaging -> Image pre-warming verified
[drain-test] Stage: config_bootstrap -> Uncordoning node 'kubebuild-lab-worker'
>> Node kubebuild-lab-worker marked Schedulable (Uncordoned)