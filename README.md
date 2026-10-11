# KubeBuild Operator: Kubernetes Bare-Metal Fleet Lifecycle Controller

A Go-based Kubernetes Operator simulating hyperscale fleet lifecycle automation (analogous to Meta's Autobuild and Crusoe Cloud's bare-metal health automation).

The operator reconciles a custom resource (`ReimageRequest`) to automate node cordoning, non-disruptive workload draining via the `policy/v1` Eviction API, out-of-band hardware resets via modern Redfish REST APIs, and automated cluster re-integration.

---

## Architecture & Lifecycle Phases

The controller implements a staged state machine:

| Stage | Phase | Action |
|---|---|---|
| `config_gen` | `Running` | Validates target node specs, image profiles, and readiness criteria. |
| `dhcp_add` | `Running` | Cordons the node (`Unschedulable: true`), evicts non-daemon workloads via Kubernetes Eviction API, and issues an HTTP POST power-cycle action to the server BMC via Redfish. |
| `imaging` | `Running` | Simulates OS provisioning, driver checks, and node recovery polling. |
| `config_bootstrap` | `Running` | Uncordons the node (`Unschedulable: false`), returning it to schedulable `Ready`. |
| `inops_set_to_built` | `Built` | Finalizes status subresource; reconciliation loop idles cleanly. |

---

## Infrastructure Topology

* **Multi-Node Cluster:** Local 3-node `kind` cluster (`control-plane`, `worker`, `worker2`).
* **Out-of-Band Remote Control:** Mock Redfish BMC container exposing standard DMTF endpoints on `:8000`.
* **Client Tooling:** Built natively on Windows PowerShell using Go 1.23, `client-go v0.31.0`, and `apimachinery`.

---

## Step-by-Step Reproduction Guide

### 1. Prerequisites
* Windows 10/11 with Docker Desktop running
* `kind.exe` and `kubectl.exe` in system path
* Go (version 1.22+)

### 2. Cluster & Mock BMC Setup
```powershell```
  * Create 3-node "kind" cluster
    kind create cluster --name kubebuild-lab --config kind-config.yaml

  * Deploy mock Redfish BMC
    kubectl apply -f [https://raw.githubusercontent.com/DMTF/Redfish-Mockup-Server/master/deploy/k8s/redfish-mockup.yaml](https://raw.githubusercontent.com/DMTF/Redfish-Mockup-Server/master/deploy/k8s/redfish-mockup.yaml)
    kubectl port-forward svc/redfish-service 8000:8000

### 2.1 Verify Topology Health & BMC Reachability
    * Verify 3-node cluster topology
      kubectl get nodes -o wide

    * Verify Redfish BMC Service Endpoint
      curl.exe -i -X POST http://localhost:8000/redfish/v1/Systems/worker-01/Actions/ComputerSystem.Reset

### 3. Deploy Custom Resource Definition (CRD)
    kubectl apply -f config/crd/bases/build.autobuild.io_reimagerequests.yaml

### 4. Deploy Sample Application
      kubectl apply -f config/workloads/sample-workload.yaml
      kubectl get pods -o wide -l app=web-service

### 5. Run the Operator
      go run main.go

### 6. Trigger Node Lifecycle
      kubectl apply -f config/samples/build_v1alpha1_reimagerequest_worker1.yaml
  
---

## Post-Mortems & Operational Learnings

Documented incident post-mortems and debugging logs encountered during development:

* [Incident 01: Redfish BMC TCP Handshake Failure (`connectex`)](docs/learning-points/01-redfish-bmc-connection-refusal.md)
* [Incident 02: Stale Controller Binary & Pod Eviction Bypass](docs/learning-points/02-stale-binary-and-eviction-bypass.md)
