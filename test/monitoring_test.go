package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// dcJobs is the subset of the Prometheus jobs deployed in dctest but not deployed in kindtest
var dcJobs = []string{
	"cke",
	"cke-etcd",
	"external-dns",
	"monitor-hw",
	"teleport",
	"bootserver-etcd",
	"node-exporter",
	"sabakan",
}
var (
	globalHealthFQDN  = testID + "-ingress-health-global.gcp0.dev-ne.co"
	bastionHealthFQDN = testID + "-ingress-health-bastion.gcp0.dev-ne.co"

	bastionPushgatewayFQDN = testID + "-pushgateway-bastion.gcp0.dev-ne.co"
	forestPushgatewayFQDN  = testID + "-pushgateway-forest.gcp0.dev-ne.co"
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

	var podName string
	It("should reply successfully", func() {
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

			_, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "exec",
				podName, "curl", "http://localhost:9090/api/v1/alerts")
			if err != nil {
				return fmt.Errorf("unable to curl :9090/api/v1/alerts, stderr: %s, err: %v", stderr, err)
			}
			return nil
		}).Should(Succeed())
	})

	It("should find endpoint", func() {
		if withKind {
			Skip("does not make sense with kindtest")
		}

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

func testPushgateway() {
	manifestBase := `apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: pushgateway-bastion
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: pushgateway-basition-tls
  routes:
    - conditions:
        - prefix: /
      services:
        - name: pushgateway
          port: 9091
      permitInsecure: true
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: pushgateway-forest
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: forest
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: pushgateway-forest-tls
  routes:
    - conditions:
        - prefix: /
      services:
        - name: pushgateway
          port: 9091
      permitInsecure: true
`

	It("should create HTTPProxy for Pushgateway", func() {
		manifest := fmt.Sprintf(manifestBase, bastionPushgatewayFQDN, forestPushgatewayFQDN)
		_, stderr, err := ExecAtWithInput(boot0, []byte(manifest), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

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

	if !withKind {
		It("should be accessed from Bastion", func() {
			By("getting the IP address of the contour LoadBalancer")
			bastionIP, err := getLoadBalancerIP("ingress-bastion", "envoy")
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() error {
				stdout, stderr, err := ExecAt(boot0,
					"curl", "-skL", "--resolve", bastionPushgatewayFQDN+":80:"+bastionIP, "http://"+bastionPushgatewayFQDN+"/-/healthy",
					"-o", "/dev/null",
				)
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}
				return nil
			}).Should(Succeed())
		})

		It("should be accessed from Forest", func() {
			forestIP, err := getLoadBalancerIP("ingress-forest", "envoy")
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() error {
				return exec.Command("sudo", "nsenter", "-n", "-t", externalPID, "curl", "--resolve", forestPushgatewayFQDN+":80:"+forestIP, forestPushgatewayFQDN+"/-/healthy", "-m", "5").Run()
			}).Should(Succeed())
		})
	}
}

func testIngressHealth() {
	It("should be deployed successfully", func() {
		By("for ingress-health (testhttpd)")
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
	})

	if !withKind {
		It("should create HTTPProxy for ingress-watcher", func() {
			manifest := fmt.Sprintf(`apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: ingress-health-global
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: global
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: ingress-health-global-tls
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
  name: ingress-health-bastion
  namespace: monitoring
  annotations:
    kubernetes.io/tls-acme: "true"
    kubernetes.io/ingress.class: bastion
spec:
  virtualhost:
    fqdn: %s
    tls:
      secretName: ingress-health-bastion-tls
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

		It("should be override configuration of ingress-watcher", func() {
			config := fmt.Sprintf(`
targetURLs:
- https://%s
- http://%s
- https://%s
- http://%s
watchInterval: 10s

pushAddr: %s
jobName: ingress-watcher-0
pushInterval: 10s
permitInsecure: true
`, bastionHealthFQDN, bastionHealthFQDN, globalHealthFQDN, globalHealthFQDN, bastionPushgatewayFQDN)
			err := ioutil.WriteFile("/etc/ingress-wacher/ingress-wacher.yaml", []byte(config), os.FileMode(0644))
			Expect(err).NotTo(HaveOccurred())
			ExecSafeAt(boot0, "sudo", "systemctl", "restart", "ingress-wacher.service")
		})

		It("should push metrics to the push-gateway", func() {
			By("requesting push-gateway server")
			Eventually(func() error {
				stdout, stderr, err := ExecAt(boot0, "curl", "-skL", "https://"+bastionPushgatewayFQDN+"/metrics")
				if err != nil {
					return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
				}

				res := string(stdout)
				for _, targetFQDN := range []string{globalHealthFQDN, bastionHealthFQDN} {
					if !strings.Contains(res, fmt.Sprintf(`ingresswatcher_http_get_successful_total{code="200 OK",path="http://%s"}`, targetFQDN)) {
						return fmt.Errorf("metric http_get_successful_total does not exist: stdout=%s, url=http://%s", stdout, targetFQDN)
					}
				}

				return nil
			}).Should(Succeed())
		})
	}
}

func testUnboundService() {
	It("should be deployed successfully", func() {
		var ip string
		By("confirming that unbound is exported")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=internet-egress",
				"get", "service/unbound-bastion", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			service := new(corev1.Service)
			err = json.Unmarshal(stdout, service)
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			if len(service.Status.LoadBalancer.Ingress) != 1 {
				return fmt.Errorf("unable to get LoadBalancer's IP address. stdout: %s, stderr: %s, err: %w", stdout, stderr, err)
			}

			ip = service.Status.LoadBalancer.Ingress[0].IP

			return nil
		}).Should(Succeed())

		if !withKind {
			By("confirming that nslookup from boot server is successfull")
			targets := []string{
				bastionPushgatewayFQDN,
				forestPushgatewayFQDN,
			}
			Eventually(func() error {
				for _, target := range targets {
					stdout, stderr, err := ExecAt(boot0, "nslookup", target, ip)
					if err != nil {
						return fmt.Errorf("target: %s, stdout: %s, stderr: %s, err: %v", target, stdout, stderr, err)
					}
				}
				return nil
			}).Should(Succeed())

			By("setting dns address to neco config")
			stdout, stderr, err := ExecAt(boot0, "neco", "config", "set", "dns", ip)
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)

			stdout, stderr, err = ExecAt(boot0, "neco", "config", "get", "dns")
			Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			Expect(string(bytes.TrimSpace(stdout))).To(Equal(ip))

			By("confirming that dns is set")
			Eventually(func() error {
				for _, target := range targets {
					stdout, stderr, err := ExecAt(boot0, "nslookup", target)
					if err != nil {
						return fmt.Errorf("target: %s, stdout: %s, stderr: %s, err: %v", target, stdout, stderr, err)
					}
				}
				return nil
			}).Should(Succeed())
		}
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
	})

	It("should have data sources and dashboards", func() {
		By("getting external IP of grafana service")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace=monitoring", "get", "services", "grafana-service", "-o=json")
		Expect(err).NotTo(HaveOccurred(), "stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		service := new(corev1.Service)
		err = json.Unmarshal(stdout, service)
		Expect(err).NotTo(HaveOccurred())
		loadBalancerIP := service.Status.LoadBalancer.Ingress[0].IP
		exposedPort := strconv.Itoa(int(service.Spec.Ports[0].Port))

		By("getting admin stats from grafana")
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "curl", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", loadBalancerIP+":"+exposedPort+"/api/admin/stats")
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
			stdout, stderr, err := ExecAt(boot0, "curl", "-u", "admin:AUJUl1K2xgeqwMdZ3XlEFc1QhgEQItODMNzJwQme", loadBalancerIP+":"+exposedPort+"/api/search?type=dash-db")
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
	var podName string

	It("should be up all scraping", func() {
		By("retrieving prometheus podName")
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
			jobName := sc.JobName
			if withKind && isDCJob(jobName) {
				continue
			}
			jobNames = append(jobNames, model.LabelName(jobName))
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

func isDCJob(job string) bool {
	for _, dcJob := range dcJobs {
		if dcJob == job {
			return true
		}
	}
	return false
}

func findTarget(job string, targets []promv1.ActiveTarget) *promv1.ActiveTarget {
	for _, t := range targets {
		if string(t.Labels["job"]) == job {
			return &t
		}
	}
	return nil
}
