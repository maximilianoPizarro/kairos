package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func kubernetesAPIBase() string {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" {
		host = "kubernetes.default.svc"
	}
	if port == "" {
		port = "443"
	}
	return fmt.Sprintf("https://%s:%s", host, port)
}

func clusterDisplayName() string {
	if n := os.Getenv("CLUSTER_NAME"); n != "" {
		return n
	}
	return "local"
}


func inClusterHTTPClient() *http.Client {
	const caPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caPEM, err := os.ReadFile(caPath); err == nil {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(caPEM) {
			transport.TLSClientConfig = &tls.Config{RootCAs: pool}
		}
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}

// listSmartScalingPoliciesFromCluster returns UI rows from SmartScalingPolicy CRs in kairos-system.
func listSmartScalingPoliciesFromCluster() ([]map[string]interface{}, error) {
	token := getServiceAccountToken()
	if token == "" {
		return nil, fmt.Errorf("no service account token")
	}

	url := kubernetesAPIBase() + "/apis/kairos.maximilianopizarro.github.io/v1alpha1/namespaces/kairos-system/smartscalingpolicies"
	client := inClusterHTTPClient()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kubernetes API %s: %s", resp.Status, string(body))
	}

	var list struct {
		Items []struct {
			Metadata struct {
				Name        string            `json:"name"`
				Namespace   string            `json:"namespace"`
				Annotations map[string]string `json:"annotations"`
				Labels      map[string]string `json:"labels"`
			} `json:"metadata"`
			Spec struct {
				Paused bool `json:"paused"`
				Target struct {
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
				} `json:"target"`
				PrometheusEndpoint string `json:"prometheusEndpoint"`
				Rules              []interface{} `json:"rules"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0, len(list.Items))
	for _, item := range list.Items {
		cluster := clusterDisplayName()
		if item.Metadata.Annotations != nil {
			if dc := item.Metadata.Annotations["kairos.io/display-cluster"]; dc != "" {
				cluster = dc
			}
		}
		if item.Metadata.Labels != nil {
			if dc := item.Metadata.Labels["kairos.io/spoke"]; dc != "" {
				cluster = dc
			}
		}
		target := item.Spec.Target.Name
		if target == "" {
			target = "unknown"
		}
		out = append(out, map[string]interface{}{
			"name":               item.Metadata.Name,
			"namespace":          item.Metadata.Namespace,
			"cluster":            cluster,
			"target":             target,
			"targetNamespace":    item.Spec.Target.Namespace,
			"rules":              len(item.Spec.Rules),
			"paused":             item.Spec.Paused,
			"metricsSource":      "Prometheus",
			"prometheusEndpoint": item.Spec.PrometheusEndpoint,
			"lastAction":         time.Now().UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
