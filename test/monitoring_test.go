package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/cybozu-go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})
}

func testPrometheus() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "statefulset/prometheus", "-o=json")
			if err != nil {
				return err
			}
			statefulSet := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, statefulSet)
			if err != nil {
				return err
			}

			if int(statefulSet.Status.ReadyReplicas) != 1 {
				return fmt.Errorf("ReadyReplicas is not 1: %d", int(statefulSet.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully", func() {
		var podName string
		Eventually(func() error {
			name, err := getPrometheusPodName()
			if err != nil {
				return err
			}
			podName = name
			return nil
		}).Should(Succeed())

		Eventually(func() error {
			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9090/api/v1/alerts")
			if err != nil {
				return fmt.Errorf("unable to curl :9090/api/v1/alerts, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())

		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9090/api/v1/targets")
			if err != nil {
				return fmt.Errorf("unable to curl :9090/api/v1/targets, stderr: %s, err: %v", stderr, err)
			}

			var response struct {
				TargetsResult promv1.TargetsResult `json:"data"`
			}
			err = json.Unmarshal(stdout, &response)
			if err != nil {
				return err
			}

			for _, target := range response.TargetsResult.Active {
				if value, ok := target.Labels["kubernetes_name"]; ok {
					if value == "prometheus-node-targets" && target.Health == promv1.HealthGood {
						return nil
					}
				}
			}
			return errors.New("cannot find accessible node target")
		}).Should(Succeed())
	})
}

func testAlertmanager() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/alertmanager", "-o=json")
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

	It("should reply successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=alertmanager", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("alertmanager pod doesn't exist")
			}
			podName := podList.Items[0].Name

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9093/-/healthy")
			if err != nil {
				return fmt.Errorf("unable to curl :9090/-/halthy, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}

func preparePushgateway() {
	manifestBase := `
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: pushgateway-bastion-test
  namespace: monitoring
  annotations:
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
  routes:
    - conditions:
        - prefix: /
      services:
        - name: pushgateway
          port: 9091
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: pushgateway-forest-test
  namespace: monitoring
  annotations:
    kubernetes.io/ingress.class: forest
spec:
  virtualhost:
    fqdn: %s
  routes:
    - conditions:
        - prefix: /
      services:
        - name: pushgateway
          port: 9091
`

	It("should create HTTPProxy for Pushgateway", func() {
		manifest := fmt.Sprintf(manifestBase, bastionPushgatewayFQDN, forestPushgatewayFQDN)
		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
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
			stdout, stderr, err := ExecAt(boot0,
				"curl", "-s", "http://"+bastionPushgatewayFQDN+"/-/healthy", "-o", "/dev/null",
			)
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
			return exec.Command("sudo", "nsenter", "-n", "-t", externalPID, "curl", "--resolve", forestPushgatewayFQDN+":80:"+forestIP, forestPushgatewayFQDN+"/-/healthy", "-m", "5").Run()
		}).Should(Succeed())
	})
}

func prepareIngressHealth() {
	It("should create HTTPProxy for ingress-watcher", func() {
		manifest := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: ingress-health-global-test
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: global
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: ingress-health-global-test-tls
  routes:
    - conditions:
        - prefix: /
      services:
        - name: ingress-health-http
          port: 80
      permitInsecure: true
      timeoutPolicy:
        response: 2m
        idle: 5m
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: ingress-health-bastion-test
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: ingress-health-bastion-test-tls
  routes:
    - conditions:
        - prefix: /
      services:
        - name: ingress-health-http
          port: 80
      permitInsecure: true
      timeoutPolicy:
        response: 2m
        idle: 5m
`, globalHealthFQDN, bastionHealthFQDN)

		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
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

			if int(deployment.Status.AvailableReplicas) != 1 {
				return fmt.Errorf("AvailableReplicas is not 1: %d", int(deployment.Status.AvailableReplicas))
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
			stdout, stderr, err := ExecAt(boot0, "curl", "-s", "http://"+bastionPushgatewayFQDN+"/metrics")
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

func prepareGrafanaOperator() {
	It("should create HTTPProxy for grafana", func() {
		manifest := fmt.Sprintf(`
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: grafana-test
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: grafana-test-tls
  routes:
    - conditions:
        - prefix: /
      services:
        - name: grafana-service
          port: 3000
      timeoutPolicy:
        response: 2m
        idle: 5m
`, grafanaFQDN)

		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
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
			stdout, stderr, err := ExecAt(boot0, "curl", "-kL", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", grafanaFQDN+"/api/admin/stats")
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

func testPrometheusMetrics() {
	It("should be up all scraping", func() {
		var podName string
		Eventually(func() error {
			name, err := getPrometheusPodName()
			if err != nil {
				return err
			}
			podName = name
			return nil
		}).Should(Succeed())

		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=prometheus", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("prometheus pod doesn't exist")
			}
			podName = podList.Items[0].Name
			return nil
		}).Should(Succeed())

		By("retrieving job_name from prometheus.yaml")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
			"get", "configmap", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		cmList := new(corev1.ConfigMapList)
		err = json.Unmarshal(stdout, cmList)
		Expect(err).NotTo(HaveOccurred())

		var promConfigFound bool

		var promConfig struct {
			ScrapeConfigs []struct {
				JobName string `json:"job_name"`
			} `json:"scrape_configs"`
		}
		for _, cm := range cmList.Items {
			if data, ok := cm.Data["prometheus.yaml"]; ok {
				err := yaml.Unmarshal([]byte(data), &promConfig)
				Expect(err).NotTo(HaveOccurred())
				promConfigFound = true
			}
		}
		Expect(promConfigFound).To(BeTrue())

		var jobNames []model.LabelName
		for _, sc := range promConfig.ScrapeConfigs {
			jobNames = append(jobNames, model.LabelName(sc.JobName))
		}

		By("checking discovered active labels and statuses")
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9090/api/v1/targets")
			if err != nil {
				return err
			}

			var response struct {
				TargetsResult promv1.TargetsResult `json:"data"`
			}
			err = json.Unmarshal(stdout, &response)
			if err != nil {
				return err
			}

			// monitor-hw job on stopped machine should be down
			const stoppedMachineInDCTest = 1
			downedMonitorHW := 0
			for _, jobName := range jobNames {
				target := findTarget(string(jobName), response.TargetsResult.Active)
				if target == nil {
					return fmt.Errorf("target is not found, job_name: %s", jobName)
				}
				if target.Health != promv1.HealthGood {
					if target.Labels["job"] != "monitor-hw" {
						return fmt.Errorf("target is not 'up', job_name: %s, health: %s", jobName, target.Health)
					}
					downedMonitorHW++
					if downedMonitorHW > stoppedMachineInDCTest {
						return fmt.Errorf("two or more monitor-hw jobs are not up; health: %s", target.Health)
					}
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should be loaded all alert rules", func() {
		var podName string
		Eventually(func() error {
			name, err := getPrometheusPodName()
			if err != nil {
				return err
			}
			podName = name
			return nil
		}).Should(Succeed())

		var expected []string
		var actual []string
		err := filepath.Walk("../monitoring/base/prometheus/alert_rules", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			str, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			var groups alertRuleGroups
			err = yaml.Unmarshal(str, &groups)
			if err != nil {
				return fmt.Errorf("failed to unmarshal %s, err: %v", path, err)
			}

			for _, g := range groups.Groups {
				for _, a := range g.Alerts {
					if len(a.Alert) != 0 {
						expected = append(expected, a.Alert)
					}
				}
			}

			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec", podName, "curl", "http://localhost:9090/api/v1/rules")
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		var response struct {
			Rules promv1.RulesResult `json:"data"`
		}
		err = json.Unmarshal(stdout, &response)
		Expect(err).NotTo(HaveOccurred())

		for _, g := range response.Rules.Groups {
			for _, r := range g.Rules {
				rule, ok := r.(promv1.AlertingRule)
				if !ok {
					continue
				}
				if len(rule.Name) != 0 {
					actual = append(actual, rule.Name)
				}
			}
		}
		sort.Strings(actual)
		sort.Strings(expected)
		Expect(len(actual)).NotTo(Equal(0))
		Expect(len(expected)).NotTo(Equal(0))
		Expect(reflect.DeepEqual(actual, expected)).To(BeTrue(),
			"\nactual   = %v\nexpected = %v", actual, expected)
	})

	It("should be loaded all record rules", func() {
		var podName string
		Eventually(func() error {
			name, err := getPrometheusPodName()
			if err != nil {
				return err
			}
			podName = name
			return nil
		}).Should(Succeed())

		var expected []string
		var actual []string
		str, err := ioutil.ReadFile("../monitoring/base/prometheus/record_rules.yaml")
		Expect(err).NotTo(HaveOccurred())

		var groups recordRuleGroups
		err = yaml.Unmarshal(str, &groups)
		Expect(err).NotTo(HaveOccurred())

		for _, g := range groups.Groups {
			for _, r := range g.Records {
				if len(r.Record) != 0 {
					expected = append(expected, r.Record)
				}
			}
		}

		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec", podName, "curl", "http://localhost:9090/api/v1/rules")
		Expect(err).NotTo(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		var response struct {
			Rules promv1.RulesResult `json:"data"`
		}
		err = json.Unmarshal(stdout, &response)
		Expect(err).NotTo(HaveOccurred())

		for _, g := range response.Rules.Groups {
			if !strings.HasSuffix(g.Name, ".records") {
				continue
			}
			for _, r := range g.Rules {
				rule, ok := r.(promv1.RecordingRule)
				if !ok {
					continue
				}
				if len(rule.Name) != 0 {
					actual = append(actual, rule.Name)
				}
			}
		}
		sort.Strings(actual)
		sort.Strings(expected)
		Expect(len(actual)).NotTo(Equal(0))
		Expect(len(expected)).NotTo(Equal(0))
		Expect(reflect.DeepEqual(actual, expected)).To(BeTrue(),
			"\nactual   = %v\nexpected = %v", actual, expected)
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

func testVMAlertmanager() {
	It("should be deployed successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "statefulset/vmalertmanager-vmalertmanager", "-o=json")
			if err != nil {
				return err
			}
			sts := new(appsv1.StatefulSet)
			err = json.Unmarshal(stdout, sts)
			if err != nil {
				return err
			}

			if int(sts.Status.ReadyReplicas) != 1 {
				return fmt.Errorf("ReadyReplicas is not 1: %d", int(sts.Status.ReadyReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should reply successfully", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalertmanager", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("vmalertmanager pod doesn't exist")
			}
			podName := podList.Items[0].Name

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9093/-/healthy")
			if err != nil {
				return fmt.Errorf("unable to curl :9093/-/halthy, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})
}

func testVMSmallsetClusterComponents() {
	const vmagentCount = 3

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

	It("should be deployed successfully (vmalert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmalert-vmalert-smallset", "-o=json")
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

	It("should be deployed successfully (vmagent)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmagent-vmagent-smallset", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != vmagentCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vmagentCount, int(deployment.Status.AvailableReplicas))
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

	It("should reply successfully (vmalert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalert,app.kubernetes.io/instance=vmalert-smallset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != 1 {
				return errors.New("vmalert pod doesn't exist")
			}
			podName := podList.Items[0].Name

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:8080/api/v1/alerts")
			if err != nil {
				return fmt.Errorf("unable to curl :8080/api/v1/alerts, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should find endpoint (vmagent)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-smallset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != vmagentCount {
				return errors.New("vmagent pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:8429/api/v1/targets")
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

				found := false
				for _, target := range response.TargetsResult.Active {
					if value, ok := target.Labels["job"]; ok {
						if value == "kubernetes-nodes" && target.Health == promv1.HealthGood {
							found = true
							break
						}
					}
				}
				if !found {
					return errors.New("cannot find target")
				}
			}
			return nil
		}).Should(Succeed())
	})
}

func testVMLargesetClusterComponents() {
	const vmstorageCount = 3
	const vmselectCount = 3
	const vminsertCount = 3
	const vmagentCount = 3
	const vmalertCount = 3

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

	It("should be deployed successfully (vmalert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmalert-vmalert-largeset", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != vmalertCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vmalertCount, int(deployment.Status.AvailableReplicas))
			}
			return nil
		}).Should(Succeed())
	})

	It("should be deployed successfully (vmagent)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "deployment/vmagent-vmagent-largeset", "-o=json")
			if err != nil {
				return err
			}
			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return err
			}

			if int(deployment.Status.AvailableReplicas) != vmagentCount {
				return fmt.Errorf("AvailableReplicas is not %d: %d", vmagentCount, int(deployment.Status.AvailableReplicas))
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

	It("should reply successfully (vmalert)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmalert,app.kubernetes.io/instance=vmalert-largeset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != vmalertCount {
				return errors.New("vmalert pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:8080/api/v1/alerts")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8080/api/v1/alerts, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})

	It("should find endpoint (vmagent)", func() {
		Eventually(func() error {
			stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring",
				"get", "pods", "--selector=app.kubernetes.io/name=vmagent,app.kubernetes.io/instance=vmagent-largeset", "-o=json")
			if err != nil {
				return err
			}
			podList := new(corev1.PodList)
			err = json.Unmarshal(stdout, podList)
			if err != nil {
				return err
			}
			if len(podList.Items) != vmagentCount {
				return errors.New("vmagent pod count mismatch")
			}
			for _, pod := range podList.Items {
				podName := pod.Name

				_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
					podName, "curl", "http://localhost:8429/api/v1/targets")
				if err != nil {
					return fmt.Errorf("unable to curl http://%s:8429/api/v1/targets, stderr: %s, err: %v", podName, stderr, err)
				}
			}
			return nil
		}).Should(Succeed())
	})
}

func findTarget(job string, targets []promv1.ActiveTarget) *promv1.ActiveTarget {
	for _, t := range targets {
		if string(t.Labels["job"]) == job {
			return &t
		}
	}
	return nil
}

func getPrometheusPodName() (string, error) {
	stdout, _, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "get", "pods", "--selector=app.kubernetes.io/name=prometheus", "-o=json")
	if err != nil {
		return "", err
	}

	podList := new(corev1.PodList)
	err = json.Unmarshal(stdout, podList)
	if err != nil {
		return "", err
	}

	if len(podList.Items) != 1 {
		return "", errors.New("prometheus pod doesn't exist")
	}
	podName := podList.Items[0].Name
	return podName, nil
}
