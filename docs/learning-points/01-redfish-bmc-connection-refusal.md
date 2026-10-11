# Incident Post-Mortem 01: Redfish BMC HTTP Connection Refused

## Overview
During the initial execution of the `ReimageRequest` reconciliation loop, the controller successfully completed the `config_gen` validation and cordoned the target worker node (`kubebuild-lab-worker`). However, the out-of-band chassis power-cycle step failed silently with a connection refusal warning, skipping the hardware reboot simulation.

---

## Error Signature
```text
>> Redfish reset call skipped (port-forward offline or unreachable): 
Post "http://localhost:8000/redfish/v1/Systems/worker-01/Actions/ComputerSystem.Reset": 
dial tcp [::1]:8000: connectex: No connection could be made because the target machine actively refused it.