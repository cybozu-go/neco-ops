package test

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sYaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

var (
	globalHealthFQDN  = testID + "-ingress-health-global.gcp0.dev-ne.co"
	bastionHealthFQDN = testID + "-ingress-health-bastion.gcp0.dev-ne.co"

	bastionPushgatewayFQDN = testID + "-pushgateway-bastion.gcp0.dev-ne.co"
	forestPushgatewayFQDN  = testID + "-pushgateway-forest.gcp0.dev-ne.co"
)

var (
	grafanaFQDN = testID + "-grafana.gcp0.dev-ne.co"
)

func testMachinesEndpoints() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			_, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "cronjob/machines-endpoints-cronjob")
			if err != nil {
				return err
			}

			return nil
		}).Should(Succeed())
	})

	It("should register endpoints", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "endpoints/prometheus-node-targets", "-o=json")
			if err != nil {
				return err
			}

			endpoints := new(corev1.Endpoints)
			err = json.Unmarshal(stdout, endpoints)
			if err != nil {
				return err
			}

			if len(endpoints.Subsets) != 1 {
				return errors.New("len(endpoints.Subsets) != 1")
			}
			if len(endpoints.Subsets[0].Addresses) == 0 {
				return errors.New("no address in endpoints")
			}
			if len(endpoints.Subsets[0].Ports) == 0 {
				return errors.New("no port in endpoints")
			}

			return nil
		}).Should(Succeed())
	})
}

func testKubeStateMetrics() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=kube-system",
				"get", "deployment/kube-state-metrics", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 2 {
				return fmt.Errorf("AvailableReplicas is not 2: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})
}

//go:embed testdata/monitoring-pushgateway.yaml
var monitoringPushgatewayYAML string

func preparePushgateway() {
	It("should create HTTPProxy for Pushgateway", func() {
		tmpl := template.Must(template.New("").Parse(monitoringPushgatewayYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})
}

func testPushgateway() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/pushgateway", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should be accessed from Bastion", func() {
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", bastionPushgatewayFQDN+":80:"+ip, "-s", "http://"+bastionPushgatewayFQDN+"/-/healthy", "-o", "/dev/null")
			if err != nil {
				log.Warn("curl failed", map[string]interface{}{
					"stdout": string(stdout),
					"stderr": string(stderr),
				})
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", string(stdout), string(stderr), err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should be accessed from Forest", func() {
		var forestIP string
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-forest", "envoy")
			if err != nil {
				return err
			}
			forestIP = ip
			return nil
		}).Should(Succeed())
		Eventually(func() error {
			return exec.Command("ip", "netns", "exec", "external", "curl", "--resolve", forestPushgatewayFQDN+":80:"+forestIP, forestPushgatewayFQDN+"/-/healthy", "-m", "5").Run()
		}).Should(Succeed())
	})
}

//go:embed testdata/monitoring-ingresshealth.yaml
var monitoringIngressHealthYAML string

func prepareIngressHealth() {
	It("should create HTTPProxy for ingress-watcher", func() {
		tmpl := template.Must(template.New("").Parse(monitoringIngressHealthYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "failed to create HTTPProxy. stderr: %s", stderr)
	})
}

func testIngressHealth() {
	It("should be reported as healthy by ingress-watcher", func() {
		By("checking ingress-health Deployment")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/ingress-health", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 2 {
				return fmt.Errorf("AvailableReplicas is not 2: %d", int(deployment.Status.AvailableReplicas))
			}

			stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", "monitoring", "get", "service", "ingress-health-http")
			if err != nil {
				return fmt.Errorf("unable to get ingress-health-http. stdout: %s, stderr: %s, err: %w", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("confirming created Certificate")
		Eventually(func() error {
			err := checkCertificate("ingress-health-global-test", "monitoring")
			if err != nil {
				return err
			}
			return checkCertificate("ingress-health-bastion-test", "monitoring")
		}).Should(Succeed())

		By("comfirming ingress-watcher configuration file")
		ingressWatcherConfPath := "/etc/ingress-watcher/ingress-watcher.yaml"
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "test", "-f", ingressWatcherConfPath)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		By("replacing ingress-watcher configuration file")
		config := fmt.Sprintf(`
targetURLs:
- https://%s
- http://%s
- https://%s
- http://%s
watchInterval: 10s

instance: 1.2.3.4
pushAddr: %s
pushInterval: 10s
permitInsecure: true
`, bastionHealthFQDN, bastionHealthFQDN, globalHealthFQDN, globalHealthFQDN, bastionPushgatewayFQDN)
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(config), "sudo", "dd", "of="+ingressWatcherConfPath)
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		ExecSafeAt(boot0, "sudo", "systemctl", "restart", "ingress-watcher.service")

		By("getting metrics from push-gateway server")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", bastionPushgatewayFQDN+":80:"+ip+", -s", "http://"+bastionPushgatewayFQDN+"/metrics")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			res := string(stdout)
			for _, targetFQDN := range []string{globalHealthFQDN, bastionHealthFQDN} {
			OUTER:
				for _, schema := range []string{"http", "https"} {
					path := fmt.Sprintf(`path="%s://%s"`, schema, targetFQDN)
					for _, line := range strings.Split(res, "\n") {
						if strings.Contains(line, "ingresswatcher_http_get_successful_total") &&
							strings.Contains(line, `code="200`) &&
							strings.Contains(line, path) {
							continue OUTER
						}
					}
					return fmt.Errorf("metric ingresswatcher_http_get_successful_total does not exist: metrics=%s, path=%s://%s", res, schema, targetFQDN)
				}
			}

			return nil
		}).Should(Succeed())
	})
}

func getLoadBalancerIP(namespace, service string) (string, error) {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "-n", namespace, "get", "service", service, "-o=json")
	if err != nil {
		return "", fmt.Errorf("unable to get %s/%s. stdout: %s, stderr: %s, err: %w", namespace, service, stdout, stderr, err)
	}
	svc := new(corev1.Service)
	err = json.Unmarshal(stdout, svc)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal %s/%s. err: %w", namespace, service, err)
	}
	if len(svc.Status.LoadBalancer.Ingress) != 1 {
		return "", fmt.Errorf("len(svc.Status.LoadBalancer.Ingress) != 1. %d", len(svc.Status.LoadBalancer.Ingress))
	}
	return svc.Status.LoadBalancer.Ingress[0].IP, nil
}

//go:embed testdata/monitoring-grafana-operator.yaml
var monitoringGrafanaYAML string

func prepareGrafanaOperator() {
	It("should create HTTPProxy for grafana", func() {
		tmpl := template.Must(template.New("").Parse(monitoringGrafanaYAML))
		buf := new(bytes.Buffer)
		err := tmpl.Execute(buf, testID)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "failed to create HTTPProxy. stderr: %s", stderr)
	})
}

func testGrafanaOperator() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/grafana-deployment", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.ReadyReplicas) != 1 {
				return fmt.Errorf("ReadyReplicas is not 1: %d", int(deployment.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())

		By("confirming created Certificate")
		Eventually(func() error {
			return checkCertificate("grafana-test", "monitoring")
		}).Should(Succeed())
	})

	It("should have data sources and dashboards", func() {
		By("getting admin stats from grafana")
		Eventually(func() error {
			ip, err := getLoadBalancerIP("ingress-bastion", "envoy")
			if err != nil {
				return err
			}
			stdout, stderr, err := ExecInNetns("external", "curl", "--resolve", grafanaFQDN+":443:"+ip, "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", "https://"+grafanaFQDN+"/api/admin/stats", "-m", "5")
			if err != nil {
				return fmt.Errorf("unable to get admin stats, stderr: %s, err: %v", stderr, err)
			}
			var adminStats struct {
				Dashboards  int `json:"dashboards"`
				Datasources int `json:"datasources"`
			}
			err = json.Unmarshal(stdout, &adminStats)
			if err != nil {
				return err
			}
			if adminStats.Datasources == 0 {
				return fmt.Errorf("no data sources")
			}
			if adminStats.Dashboards == 0 {
				return fmt.Errorf("no dashboards")
			}
			return nil
		}).Should(Succeed())

		By("confirming all dashboards are successfully registered")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "curl", "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", grafanaFQDN+"/api/search?type=dash-db")
			if err != nil {
				return fmt.Errorf("unable to get dashboards, stderr: %s, err: %v", stderr, err)
			}
			var dashboards []struct {
				ID int `json:"id"`
			}
			err = json.Unmarshal(stdout, &dashboards)
			if err != nil {
				return err
			}

			// NOTE: expectedNum is the number of files under monitoring/base/grafana/dashboards
			if len(dashboards) != numGrafanaDashboard {
				return fmt.Errorf("len(dashboards) should be %d: %d", numGrafanaDashboard, len(dashboards))
			}
			return nil
		}).Should(Succeed())
	})
}

func testVictoriaMetricsOperator() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/victoriametrics-operator", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 2 {
				return fmt.Errorf("AvailableReplicas is not 2: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})
}

type vmSetType struct {
	small        bool
	name         string
	vmamCount    int
	vmagentCount int
	vmalertCount int
}

// shrinked version of VMRule
type VMRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              struct {
		Groups []struct {
			Name  string `json:"name"`
			Rules []struct {
				Alert string `json:"alert"`
			} `json:"rules"`
		} `json:"groups"`
	} `json:"spec"`
}

// shrinked version of result of vmalert /api/v1/groups API
type VMAlertAPIV1GroupsResult struct {
	Data struct {
		Groups []struct {
			Name          string `json:"name"`
			AlertingRules []struct {
				Name string `json:"name"`
			} `json:"alerting_rules"`
		} `json:"groups"`
	} `json:"data"`
}

func testVMCommonClusterComponents(setType vmSetType) {
	It("should be deployed successfully (vmalertmanager)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "statefulset/vmalertmanager-vmalertmanager-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			sts := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, sts)
			if err != nil {
				return err
			}

			if int(sts.Status.ReadyReplicas) != setType.vmamCount {
				return fmt.Errorf("ReadyReplicas is not %d: %d", setType.vmamCount, int(sts.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully (vmalertmanager)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalertmanager,app.kubernetes.io/instance=vmalertmanager-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmamCount {
				return errors.New("vmalertmanager pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:9093/-/healthy")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:9093/-/halthy, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmalert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmalert-vmalert-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != setType.vmalertCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", setType.vmalertCount, int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmagent)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmagent-vmagent-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != setType.vmagentCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", setType.vmagentCount, int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully (vmalert)", func() {
		By("reading VMRules")
		expected := []string{}
		err := filepath.Walk("../monitoring/base/victoriametrics/rules", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := k8sYaml.NewYAMLReader(bufio.NewReader(file))
			for {
				data, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("failed to read yaml: %v", err)
				}
				var r VMRule
				yaml.Unmarshal(data, &r)
				if r.Kind != "VMRule" {
					continue
				}
				if setType.small && r.Labels["smallset"] != "true" {
					continue
				}
				for _, group := range r.Spec.Groups {
					for _, rule := range group.Rules {
						expected = append(expected, rule.Alert)
					}
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		sort.Strings(expected)

		By("checking vmalerts")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalert,app.kubernetes.io/instance=vmalert-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmalertCount {
				return errors.New("vmalert pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:8080/api/v1/groups")
				if err != nil {
					return fmt.Errorf("unable to curl :8080/api/v1/groups, stderr: %s, err: %v", stderr, err)
				}
				var r VMAlertAPIV1GroupsResult
				err = json.Unmarshal(stdout, &r)
				if err != nil {
					return err
				}
				actual := []string{}
				for _, group := range r.Data.Groups {
					for _, rule := range group.AlertingRules {
						if len(rule.Name) != 0 {
							actual = append(actual, rule.Name)
						}
					}
				}
				sort.Strings(actual)
				if !reflect.DeepEqual(actual, expected) {
					return fmt.Errorf("vmalert does not load all rules actual=%v, expected=%v",
						actual, expected)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should find endpoint (vmagent)", func() {
		By("reading scraping resources")
		jobNames := []string{}
		err := filepath.Walk("../monitoring/base/victoriametrics/rules", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := k8sYaml.NewYAMLReader(bufio.NewReader(file))
			for {
				data, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("failed to read yaml: %v", err)
				}
				var r VMScrapeOrRule
				yaml.Unmarshal(data, &r)
				var relabelConfigs [][]RelabelConfig
				switch r.Kind {
				case "VMServiceScrape":
					for _, ep := range r.Spec.ServiceScrapeEndpoints {
						relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
					}
				case "VMPodScrape":
					for _, ep := range r.Spec.PodScrapeEndpoints {
						relabelConfigs = append(relabelConfigs, ep.RelabelConfigs)
					}
				case "VMNodeScrape":
					relabelConfigs = append(relabelConfigs, r.Spec.NodeScrapeRelabelConfigs)
				case "VMProbe":
				default:
					continue
				}
				if setType.small && r.Labels["smallset"] != "true" {
					continue
				}

				for _, rcs := range relabelConfigs {
					for _, rc := range rcs {
						if rc.Action == "" && rc.TargetLabel == "job" && rc.Replacement != "" && !strings.Contains(rc.Replacement, "/") {
							jobNames = append(jobNames, rc.Replacement)
						}
					}
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("checking vmagents")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-"+setType.name, "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != setType.vmagentCount {
				return errors.New("vmagent pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					"-c", "vmagent", podName, "--",
					"curl", "http://localhost:8429/api/v1/targets")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8429/api/v1/targets, stderr: %s, err: %v", podName, stderr, err)
				}

				var response struct {
					TargetsResult promv1.TargetsResult `json:"data"`
				}
				err = json.Unmarshal(stdout, &response)
				if err != nil {
					return err
				}

				const stoppedMachinesInDCTest = 1
				downedMonitorHW := 0
				for _, jobName := range jobNames {
					targets := findTargets(string(jobName), response.TargetsResult.Active)
					if len(targets) == 0 {
						return fmt.Errorf("target is not found, job_name: %s", jobName)
					}
					for _, target := range targets {
						if target.Health != promv1.HealthGood {
							if target.Labels["job"] != "monitor-hw" {
								return fmt.Errorf("target is not 'up', job_name: %s, health: %s", jobName, target.Health)
							}
							downedMonitorHW++
							if downedMonitorHW > stoppedMachinesInDCTest {
								return fmt.Errorf("too many monitor-hw jobs are down; health: %s", target.Health)
							}
						}
					}
				}
			}
			return nil
		}).Should(Succeed())
	})

}

func testVMSmallsetClusterComponents() {
	testVMCommonClusterComponents(vmSetType{
		small:        true,
		name:         "smallset",
		vmamCount:    1,
		vmalertCount: 1,
		vmagentCount: 1,
	})

	It("should be deployed successfully (vmsingle)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmsingle-vmsingle-smallset", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully (vmsingle)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmsingle,app.kubernetes.io/instance=vmsingle-smallset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("vmsingle pod doesn't exist")
			}
			podName := podList.Items[0].Name

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:8429/api/v1/labels")
			if err != nil {
				return fmt.Errorf("unable to curl :8429/api/v1/labels, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}

func testVMLargesetClusterComponents() {
	const vmstorageCount = 3
	const vmselectCount = 3
	const vminsertCount = 3

	testVMCommonClusterComponents(vmSetType{
		small:        false,
		name:         "largeset",
		vmamCount:    3,
		vmalertCount: 3,
		vmagentCount: 3,
	})

	It("should be deployed successfully (vmstorage)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "statefulset/vmstorage-vmcluster-largeset", "-o=json")
			if err != nil {
				return err
			}
			statefulSet := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, statefulSet)
			if err != nil {
				return err
			}

			if int(statefulSet.Status.ReadyReplicas) != vmstorageCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vmstorageCount, int(statefulSet.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmselect)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "statefulset/vmselect-vmcluster-largeset", "-o=json")
			if err != nil {
				return err
			}
			statefulSet := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, statefulSet)
			if err != nil {
				return err
			}

			if int(statefulSet.Status.ReadyReplicas) != vmselectCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vmselectCount, int(statefulSet.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vminsert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vminsert-vmcluster-largeset", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != vminsertCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vminsertCount, int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully (vmselect)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmselect,app.kubernetes.io/instance=vmcluster-largeset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != vmselectCount {
				return errors.New("vmselect pod count mistatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:8481/select/0/prometheus/api/v1/labels")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8429/select/0/prometheus/api/v1/labels, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})
}

func findTargets(job string, targets []promv1.ActiveTarget) []*promv1.ActiveTarget {
	ret := []*promv1.ActiveTarget{}
	for _, t := range targets {
		if string(t.Labels["job"]) == job {
			ret = append(ret, &t)
		}
	}
	return ret
}
