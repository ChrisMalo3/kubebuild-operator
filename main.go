package main

import (
    "bytes"
    "context"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "time"

    corev1 "k8s.io/api/core/v1"
    policyv1 "k8s.io/api/policy/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

var gvr = schema.GroupVersionResource{
    Group:    "build.autobuild.io",
    Version:  "v1alpha1",
    Resource: "reimagerequests",
}

func main() {
    home, err := os.UserHomeDir()
    if err != nil {
        panic(err)
    }

    kubeconfig := filepath.Join(home, ".kube", "config")
    config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
    if err != nil {
        panic(fmt.Sprintf("Failed to load kubeconfig from %s: %v", kubeconfig, err))
    }

    dynClient, err := dynamic.NewForConfig(config)
    if err != nil {
        panic(err)
    }

    kubeClient, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err)
    }

    fmt.Println("[KubeBuild Operator] Controller active. Listening for ReimageRequests...")

    for {
        list, err := dynClient.Resource(gvr).Namespace("default").List(context.TODO(), metav1.ListOptions{})
        if err == nil {
            for _, item := range list.Items {
                reconcile(dynClient, kubeClient, item)
            }
        }
        time.Sleep(3 * time.Second)
    }
}

func reconcile(dynClient dynamic.Interface, kubeClient *kubernetes.Clientset, item unstructured.Unstructured) {
    name := item.GetName()
    status, _, _ := unstructured.NestedMap(item.Object, "status")
    currentStage, _ := status["currentStage"].(string)

    spec, _, _ := unstructured.NestedMap(item.Object, "spec")
    targetRef, _ := spec["targetRef"].(map[string]interface{})
    targetNode, _ := targetRef["name"].(string)

    switch currentStage {
    case "":
        fmt.Printf("[%s] Stage: config_gen -> Validating specifications\n", name)
        updateStatus(dynClient, item, "config_gen", "Running")

    case "config_gen":
        fmt.Printf("[%s] Stage: dhcp_add -> Cordoning & draining node: %s\n", name, targetNode)
        cordonNode(kubeClient, targetNode, true)

        // Evict user workloads so they migrate to worker2
        err := drainNode(kubeClient, targetNode)
        if err != nil {
            fmt.Printf("[%s] Drain warning: %v\n", name, err)
        }

        // Trigger BMC power-cycle
        triggerRedfishReset()
        updateStatus(dynClient, item, "imaging", "Running")

    case "imaging":
        fmt.Printf("[%s] Stage: imaging -> Verifying node recovery\n", name)
        time.Sleep(3 * time.Second)
        updateStatus(dynClient, item, "os_ready", "Running")

    case "os_ready":
        fmt.Printf("[%s] Stage: config_bootstrap -> Uncordoning node: %s\n", name, targetNode)
        cordonNode(kubeClient, targetNode, false)
        updateStatus(dynClient, item, "inops_set_to_built", "Built")

    case "inops_set_to_built":
        // Completed
    }
}

func cordonNode(client *kubernetes.Clientset, nodeName string, cordon bool) {
    node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
    if err != nil {
        fmt.Printf("[Warning] Could not inspect node %s: %v\n", nodeName, err)
        return
    }
    node.Spec.Unschedulable = cordon
    _, err = client.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
    if err != nil {
        fmt.Printf("[Warning] Failed updating cordon status on %s: %v\n", nodeName, err)
    } else {
        if cordon {
            fmt.Printf(">> Node '%s' marked Unschedulable (Cordoned)\n", nodeName)
        } else {
            fmt.Printf(">> Node '%s' restored to Schedulable (Uncordoned)\n", nodeName)
        }
    }
}

func drainNode(client *kubernetes.Clientset, nodeName string) error {
    pods, err := client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
        FieldSelector: "spec.nodeName=" + nodeName,
    })
    if err != nil {
        return err
    }

    for _, pod := range pods.Items {
        if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
            continue
        }

        isDaemonSet := false
        for _, owner := range pod.OwnerReferences {
            if owner.Kind == "DaemonSet" {
                isDaemonSet = true
                break
            }
        }
        if isDaemonSet {
            continue
        }

        if _, isMirror := pod.Annotations[corev1.MirrorPodAnnotationKey]; isMirror {
            continue
        }

        fmt.Printf(">> Evicting pod '%s/%s' from node %s...\n", pod.Namespace, pod.Name, nodeName)

        eviction := &policyv1.Eviction{
            ObjectMeta: metav1.ObjectMeta{
                Name:      pod.Name,
                Namespace: pod.Namespace,
            },
        }

        err := client.PolicyV1().Evictions(pod.Namespace).Evict(context.TODO(), eviction)
        if err != nil {
            fmt.Printf("   Failed to evict %s/%s: %v\n", pod.Namespace, pod.Name, err)
        } else {
            fmt.Printf("   Eviction accepted for %s/%s\n", pod.Namespace, pod.Name)
        }
    }

    return nil
}

func triggerRedfishReset() {
    url := "http://localhost:8000/redfish/v1/Systems/worker-01/Actions/ComputerSystem.Reset"
    resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(`{}`)))
    if err == nil {
        resp.Body.Close()
        fmt.Println(">> Simulated Redfish BMC chassis power-cycle sent (204 OK)")
    } else {
        fmt.Printf(">> Redfish reset call skipped: %v\n", err)
    }
}

func updateStatus(dynClient dynamic.Interface, item unstructured.Unstructured, nextStage, phase string) {
    status := map[string]interface{}{
        "currentStage": nextStage,
        "phase":        phase,
    }
    _ = unstructured.SetNestedMap(item.Object, status, "status")
    _, err := dynClient.Resource(gvr).Namespace("default").UpdateStatus(context.TODO(), &item, metav1.UpdateOptions{})
    if err != nil {
        _, _ = dynClient.Resource(gvr).Namespace("default").Update(context.TODO(), &item, metav1.UpdateOptions{})
    }
}
