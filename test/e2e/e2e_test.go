/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/abdullah599/namespace-quota-operator/api/v1alpha1"
	"github.com/abdullah599/namespace-quota-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "namespace-quota-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "namespace-quota-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "namespace-quota-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "namespace-quota-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=namespace-quota-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("validating that the ServiceMonitor for Prometheus is applied in the namespace")
			cmd = exec.Command("kubectl", "get", "ServiceMonitor", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "ServiceMonitor should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"namespace-quota-operator-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"namespace-quota-operator-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// Test case 1: Apply quota profile and create resource quota and limit range for already created labeled namespace
		It("should apply quota profile and create quota and limit range for already created labeled namespace", func() {
			By("creating a namespace with a label")
			cmd := exec.Command("kubectl", "create", "namespace", "ns-with-label")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace with label")

			cmd = exec.Command("kubectl", "label", "ns", "ns-with-label", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with test-label")

			By("creating a quotaProfile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply quotaProfile")

			By("validating that the namespace has the label")
			cmd = exec.Command("kubectl", "get", "ns", "ns-with-label", "-o", "jsonpath={.metadata.labels}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(v1alpha1.QuotaProfileLabelKey))

			By("checking if ResourceQuota was created")
			cmd = exec.Command("kubectl", "get", "resourcequota", "-n", "ns-with-label", "-o", "json")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var rq struct {
				Items []struct {
					Spec struct {
						Hard map[string]string `json:"hard"`
					} `json:"spec"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(output), &rq)
			Expect(err).NotTo(HaveOccurred())
			Expect(rq.Items).To(HaveLen(1))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "1"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "1Gi"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("limits.cpu", "2"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("limits.memory", "2Gi"))

			By("checking if LimitRange was created")
			cmd = exec.Command("kubectl", "get", "limitrange", "-n", "ns-with-label", "-o", "json")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var lr struct {
				Items []struct {
					Spec struct {
						Limits []struct {
							Default        map[string]string `json:"default"`
							DefaultRequest map[string]string `json:"defaultRequest"`
							Max            map[string]string `json:"max"`
							Min            map[string]string `json:"min"`
							Type           string            `json:"type"`
						} `json:"limits"`
					} `json:"spec"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(output), &lr)
			Expect(err).NotTo(HaveOccurred())
			Expect(lr.Items).To(HaveLen(1))
			Expect(lr.Items[0].Spec.Limits).To(HaveLen(1))
			Expect(lr.Items[0].Spec.Limits[0].Type).To(Equal("Container"))
			Expect(lr.Items[0].Spec.Limits[0].Default).To(HaveKeyWithValue("cpu", "500m"))
			Expect(lr.Items[0].Spec.Limits[0].DefaultRequest).To(HaveKeyWithValue("cpu", "500m"))
			Expect(lr.Items[0].Spec.Limits[0].Max).To(HaveKeyWithValue("cpu", "1"))
			Expect(lr.Items[0].Spec.Limits[0].Min).To(HaveKeyWithValue("cpu", "100m"))
		})

		// Test case 2: Apply quota profile and create resource quota and limit range for newly created labeled namespace
		It("should apply quota profile and create quota and limit range for newly created labeled namespace", func() {
			By("creating a namespace with a label")
			cmd := exec.Command("kubectl", "create", "namespace", "ns2-with-label")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace with label")

			By("applying the quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply quotaProfile")

			By("adding the label to the namespace")
			cmd = exec.Command("kubectl", "label", "ns", "ns2-with-label", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with test-label")

			By("checking if ResourceQuota was created")
			cmd = exec.Command("kubectl", "get", "resourcequota", "-n", "ns2-with-label", "-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var rq struct {
				Items []struct {
					Spec struct {
						Hard map[string]string `json:"hard"`
					} `json:"spec"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(output), &rq)
			Expect(err).NotTo(HaveOccurred())
			Expect(rq.Items).To(HaveLen(1))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "1"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "1Gi"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("limits.cpu", "2"))
			Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("limits.memory", "2Gi"))

			By("checking if LimitRange was created")
			cmd = exec.Command("kubectl", "get", "limitrange", "-n", "ns-with-label", "-o", "json")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var lr struct {
				Items []struct {
					Spec struct {
						Limits []struct {
							Default        map[string]string `json:"default"`
							DefaultRequest map[string]string `json:"defaultRequest"`
							Max            map[string]string `json:"max"`
							Min            map[string]string `json:"min"`
							Type           string            `json:"type"`
						} `json:"limits"`
					} `json:"spec"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(output), &lr)
			Expect(err).NotTo(HaveOccurred())
			Expect(lr.Items).To(HaveLen(1))
			Expect(lr.Items[0].Spec.Limits).To(HaveLen(1))
			Expect(lr.Items[0].Spec.Limits[0].Type).To(Equal("Container"))
			Expect(lr.Items[0].Spec.Limits[0].Default).To(HaveKeyWithValue("cpu", "500m"))
			Expect(lr.Items[0].Spec.Limits[0].DefaultRequest).To(HaveKeyWithValue("cpu", "500m"))
			Expect(lr.Items[0].Spec.Limits[0].Max).To(HaveKeyWithValue("cpu", "1"))
			Expect(lr.Items[0].Spec.Limits[0].Min).To(HaveKeyWithValue("cpu", "100m"))

		})

		// Test case 3: Update quota profile and verify that resource quota and limit range are updated
		It("should update resource quota and limit range when quota profile is updated", func() {
			By("applying quota profile")
			cmd := exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply quotaProfile")

			By("creating a namespace with the matching label")
			cmd = exec.Command("kubectl", "create", "namespace", "ns-update-test")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			cmd = exec.Command("kubectl", "label", "ns", "ns-update-test", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

			By("verifying initial resource quota values")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-update-test", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var rq struct {
					Items []struct {
						Spec struct {
							Hard map[string]string `json:"hard"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &rq)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rq.Items).To(HaveLen(1))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "1"))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "1Gi"))
			}).Should(Succeed())

			time.Sleep(1 * time.Second)
			By("updating the quota profile with new values")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-updated.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to update quotaProfile")

			By("verifying that the resource quota is updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-update-test", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var rq struct {
					Items []struct {
						Spec struct {
							Hard map[string]string `json:"hard"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &rq)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rq.Items).To(HaveLen(1))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "2"))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "2Gi"))
			}).Should(Succeed())

			By("verifying that the limit range is updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "limitrange", "-n", "ns-update-test", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var lr struct {
					Items []struct {
						Spec struct {
							Limits []struct {
								Default        map[string]string `json:"default"`
								DefaultRequest map[string]string `json:"defaultRequest"`
								Max            map[string]string `json:"max"`
								Min            map[string]string `json:"min"`
								Type           string            `json:"type"`
							} `json:"limits"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &lr)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(lr.Items).To(HaveLen(1))
				g.Expect(lr.Items[0].Spec.Limits[0].Default).To(HaveKeyWithValue("cpu", "750m"))
				g.Expect(lr.Items[0].Spec.Limits[0].Max).To(HaveKeyWithValue("cpu", "2"))
			}).Should(Succeed())
		})

		// Test case 4: Apply quota profile with name selector and check precedence
		It("should apply name-based quota profile with higher precedence", func() {
			By("creating a namespace")
			cmd := exec.Command("kubectl", "create", "namespace", "ns-name-selector")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			By("applying the label to the namespace")
			cmd = exec.Command("kubectl", "label", "ns", "ns-name-selector", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

			By("applying label-based quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply label-based quotaProfile")

			time.Sleep(1 * time.Second)
			By("applying name-based quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-name-selector.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply name-based quotaProfile")

			By("verifying that the name-based quota profile was applied (has precedence)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-name-selector", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var rq struct {
					Items []struct {
						Spec struct {
							Hard map[string]string `json:"hard"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &rq)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rq.Items).To(HaveLen(1))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "3"))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "3Gi"))
			}).Should(Succeed())
		})

		// Test case 5: Fallback to label-based quota profile when name-based quota profile is deleted
		It("should fallback to label-based quota profile when name-based quota profile is deleted", func() {
			By("creating a namespace")
			cmd := exec.Command("kubectl", "create", "namespace", "ns-fallback")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			By("applying the label to the namespace")
			cmd = exec.Command("kubectl", "label", "ns", "ns-fallback", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

			By("applying label-based quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply label-based quotaProfile")

			time.Sleep(1 * time.Second)
			By("applying name-based quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-name-fallback.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply name-based quotaProfile")

			By("verifying name-based quota profile is applied")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-fallback", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var rq struct {
					Items []struct {
						Spec struct {
							Hard map[string]string `json:"hard"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &rq)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rq.Items).To(HaveLen(1))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "4"))
			}).Should(Succeed())

			By("deleting the name-based quota profile")
			cmd = exec.Command("kubectl", "delete", "-f", "./test/testdata/qp-name-fallback.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete name-based quotaProfile")

			By("verifying fallback to label-based quota profile")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-fallback", "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var rq struct {
					Items []struct {
						Spec struct {
							Hard map[string]string `json:"hard"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &rq)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(rq.Items).To(HaveLen(1))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.cpu", "1"))
				g.Expect(rq.Items[0].Spec.Hard).To(HaveKeyWithValue("requests.memory", "1Gi"))
			}).Should(Succeed())
		})

		// Test case 6: Verify that user cannot update or delete managed resources
		It("should prevent users from updating or deleting managed resources", func() {
			By("creating a namespace with a label")
			cmd := exec.Command("kubectl", "create", "namespace", "ns-protected")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			By("applying the quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply quotaProfile")

			By("labeling namespace to apply quota profile")
			cmd = exec.Command("kubectl", "label", "ns", "ns-protected", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

			By("waiting for resource quota to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-protected")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			By("attempting to update the managed resource quota")
			cmd = exec.Command("kubectl", "label", "resourcequota",
				"default-quotaprofile-sample-1-0-rq",
				"--namespace", "ns-protected",
				"env=dev")
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Updating managed ResourceQuota should be blocked")
			Expect(output).To(ContainSubstring("admission webhook"), "Expected admission webhook error")

			By("attempting to update the managed limit range")
			cmd = exec.Command("kubectl", "label", "limitrange",
				"default-quotaprofile-sample-1-0-lr",
				"--namespace", "ns-protected",
				"env=dev")
			output, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Updating managed LimitRange should be blocked")
			Expect(output).To(ContainSubstring("admission webhook"), "Expected admission webhook error")

			By("attempting to delete the managed resource quota")
			cmd = exec.Command("kubectl", "delete", "resourcequota",
				"default-quotaprofile-sample-1-0-rq",
				"--namespace", "ns-protected")
			output, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Deleting managed ResourceQuota should be blocked")
			Expect(output).To(ContainSubstring("admission webhook"), "Expected admission webhook error")
		})

		// Test case 7: Delete quota profile and verify resources are cleaned up
		It("should remove labels, resource quotas, and limit ranges when quota profile is deleted", func() {
			By("creating a namespace")
			cmd := exec.Command("kubectl", "create", "namespace", "ns-cleanup")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			By("applying quota profile")
			cmd = exec.Command("kubectl", "apply", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply quotaProfile")

			By("labeling namespace to apply quota profile")
			cmd = exec.Command("kubectl", "label", "ns", "ns-cleanup", "env=dev")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace")

			By("verifying resources were created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-cleanup")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ResourceQuota should exist")

				cmd = exec.Command("kubectl", "get", "limitrange", "-n", "ns-cleanup")
				_, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "LimitRange should exist")
			}).Should(Succeed())

			By("deleting the quota profile")
			cmd = exec.Command("kubectl", "delete", "-f", "./test/testdata/qp-1.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete quotaProfile")

			By("verifying that resources are removed")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "resourcequota", "-n", "ns-cleanup")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("No resources found"), "ResourceQuota should be deleted")

				cmd = exec.Command("kubectl", "get", "limitrange", "-n", "ns-cleanup")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("No resources found"), "LimitRange should be deleted")

				cmd = exec.Command("kubectl", "get", "ns", "ns-cleanup", "-o", "jsonpath={.metadata.labels}")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(ContainSubstring(v1alpha1.QuotaProfileLabelKey),
					"QuotaProfile label should be removed")
			}).Should(Succeed())
		})

	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
