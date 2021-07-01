package test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

//go:embed testdata/storage-load.yaml
var storageLoadYAML []byte

func prepareLoadPods() {
	It("should deploy pods", func() {
		stdout, stderr, err := ExecAtWithInput(boot0, storageLoadYAML, "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl",
				"get", "deployment", "addload-for-ss", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", stdout, err)
			}

			if deployment.Status.AvailableReplicas != 2 {
				return fmt.Errorf("addload-for-ss deployment's AvailableReplicas is not 2: %d", int(deployment.Status.AvailableReplicas))
			}

			return nil
		}).Should(Succeed())
	})
}

//go:embed testdata/storage-obc.yaml
var storageOBCYAML []byte

//go:embed testdata/storage-rbd.yaml
var storageRBDYAML string

func prepareRookCeph() {
	It("should apply a OBC resource and a POD for testRookRGW", func() {
		_, stderr, err := ExecAtWithInput(boot0, storageOBCYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	It("should create a POD for testRookRBD", func() {
		tmpl := template.Must(template.New("").Parse(storageRBDYAML))
		for _, storageClassName := range []string{"ceph-hdd-block", "ceph-ssd-block", "ceph-poc-block"} {
			buf := new(bytes.Buffer)
			err := tmpl.Execute(buf, storageClassName)
			Expect(err).NotTo(HaveOccurred())

			_, stderr, err := ExecAtWithInput(boot0, buf.Bytes(), "kubectl", "apply", "-f", "-")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		}
	})
}

func testRookOperator() {
	nss := []string{"ceph-hdd", "ceph-ssd", "ceph-poc"}
	for _, ns := range nss {
		By("checking rook-ceph-operator Deployment for "+ns, func() {
			Eventually(func() error {
				stdout, _, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment/rook-ceph-operator", "-o=json")
				if err != nil {
					return err
				}

				deploy := new(appsv1.Deployment)
				err = json.Unmarshal(stdout, deploy)
				if err != nil {
					return err
				}

				if deploy.Status.AvailableReplicas != 1 {
					return fmt.Errorf("rook operator deployment's AvailableReplicas is not 1: %d", int(deploy.Status.AvailableReplicas))
				}
				return nil
			}).Should(Succeed())
		})

		By("checking ceph-tools Deployment for "+ns, func() {
			Eventually(func() error {
				stdout, _, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment/rook-ceph-tools", "-o=json")
				if err != nil {
					return err
				}

				deploy := new(appsv1.Deployment)
				err = json.Unmarshal(stdout, deploy)
				if err != nil {
					return err
				}

				if deploy.Status.AvailableReplicas != 1 {
					return fmt.Errorf("rook ceph tools deployment's AvailableReplicas is not 1: %d", int(deploy.Status.AvailableReplicas))
				}

				stdout, _, err = ExecAt(boot0, "kubectl", "get", "pod", "--namespace="+ns, "-l", "app=rook-ceph-tools", "-o=json")
				if err != nil {
					return err
				}

				pods := new(corev1.PodList)
				err = json.Unmarshal(stdout, pods)
				if err != nil {
					return err
				}

				podName := pods.Items[0].Name
				_, _, err = ExecAt(boot0, "kubectl", "exec", "--namespace="+ns, podName, "--", "ceph", "status")
				if err != nil {
					return err
				}
				return nil
			}).Should(Succeed())
		})
	}
}

func testClusterStable() {
	nss := []string{"ceph-hdd", "ceph-ssd", "ceph-poc"}
	for _, ns := range nss {
		By("checking stability of rook/ceph cluster "+ns, func() {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "deployment/rook-ceph-operator", "-o=json")
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

			deploy := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deploy)
			Expect(err).ShouldNot(HaveOccurred(), "json=%s", stdout)

			imageString := deploy.Spec.Template.Spec.Containers[0].Image
			re := regexp.MustCompile(`:(.+)\.[\d]+$`)
			group := re.FindSubmatch([]byte(imageString))
			expectRookVersion := "v" + string(group[1])

			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "cephcluster", ns, "-o", "jsonpath='{.spec.mon.count}'")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			num_mon_expected, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)

			stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+ns,
				"get", "cephcluster", ns, "-o", "jsonpath='{.spec.storage.storageClassDeviceSets[*].count}'")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			num_osd_list := strings.Fields(string(stdout))
			num_osd_expected := 0
			for _, num_osd := range num_osd_list {
				num, err := strconv.Atoi(strings.TrimSpace(string(num_osd)))
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
				num_osd_expected += num
			}

			num_rgw_expected := 0
			if ns == "ceph-hdd" || ns == "ceph-poc" {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "cephobjectstore", "-o", "jsonpath='{.items[*].spec.gateway.instances}'")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
				nums := strings.Fields(string(stdout))
				for _, num := range nums {
					n, err := strconv.Atoi(num)
					Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
					num_rgw_expected += n
				}
			}

			By("checking deployments versions are equal to the requiring")
			Eventually(func() error {
				// Confirm deployment version and pod available counts.
				stdout, _, err = ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "deployment", "-o=json")
				if err != nil {
					return err
				}

				deployments := new(appsv1.DeploymentList)
				err = json.Unmarshal(stdout, deployments)
				if err != nil {
					return err
				}

				var num_mon, num_osd, num_rgw int
				for _, deployment := range deployments.Items {
					switch deployment.Labels["app"] {
					case "rook-ceph-mon":
						num_mon++
					case "rook-ceph-osd":
						num_osd++
					case "rook-ceph-rgw":
						num_rgw++
					}

					rookVersion, ok := deployment.Labels["rook-version"]
					// Some Deployments like rook-ceph-operator and rook-ceph-tools do not have "rook-version" label,
					// so skip the check of "rook-version" for such Deployments.
					// This assumes that the operator never misses labeling to the Deployments which need to be labeled.
					if ok && !strings.HasPrefix(rookVersion, expectRookVersion) {
						return fmt.Errorf("missing deployment rook version: version=%s name=%s ns=%s", rookVersion, deployment.Name, deployment.Namespace)
					}

					if deployment.Spec.Replicas == nil {
						return fmt.Errorf("deployment's spec.replicas == nil: name=%s ns=%s", deployment.Name, deployment.Namespace)
					}
					if deployment.Status.AvailableReplicas != *deployment.Spec.Replicas {
						message := fmt.Sprintf("rook's deployment's AvailableReplicas is not expected: name=%s ns=%s %d/%d",
							deployment.Name, deployment.Namespace, int(deployment.Status.AvailableReplicas), *deployment.Spec.Replicas)
						fmt.Fprintln(GinkgoWriter, message)
						return fmt.Errorf(message)
					}
				}

				if num_mon != num_mon_expected {
					return fmt.Errorf("number of monitors is %d, expected is %d", num_mon, num_mon_expected)
				}
				if num_osd != num_osd_expected {
					return fmt.Errorf("number of OSDs is %d, expected is %d", num_osd, num_osd_expected)
				}
				if num_rgw != num_rgw_expected {
					return fmt.Errorf("number of RGWs is %d, expected is %d", num_rgw, num_rgw_expected)
				}

				return nil
			}).Should(Succeed())

			By("checking pods statuses are equal to running or job statuses are equal to succeeded")
			Eventually(func() error {
				// Show pod status.
				stdout, _, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "pod", "-o=json")
				if err != nil {
					return err
				}

				pods := new(corev1.PodList)
				err = json.Unmarshal(stdout, pods)
				if err != nil {
					return err
				}

				for _, pod := range pods.Items {
					if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
						return fmt.Errorf("pod status is not running: ns=%s name=%s time=%s", pod.Namespace, pod.Name, time.Now())
					}
				}

				return nil
			}).Should(Succeed())
		})
	}
}

func testDaemonPodsSpread(daemonName, appLabel, cephClusterNamespace string, expectDaemonCount, expectDaemonCountOnNode, expectDaemonCountInRack int) {
	By(fmt.Sprintf("checking %s Pods for %s are spread", daemonName, cephClusterNamespace), func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", "node-role.kubernetes.io/cs=true", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		nodes := new(corev1.NodeList)
		err = json.Unmarshal(stdout, nodes)
		Expect(err).ShouldNot(HaveOccurred())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+cephClusterNamespace,
			"get", "pod", "-l", appLabel, "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		pods := new(corev1.PodList)
		err = json.Unmarshal(stdout, pods)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(pods.Items).To(HaveLen(expectDaemonCount))

		nodeCounts := make(map[string]int)
		for _, pod := range pods.Items {
			nodeCounts[pod.Spec.NodeName]++
		}
		for node, count := range nodeCounts {
			Expect(count).To(Equal(expectDaemonCountOnNode), "node=%s, count=%d", node, count)
		}

		rackCounts := make(map[string]int)
		for _, node := range nodes.Items {
			if nodeCounts[node.Name] != 0 {
				rackCounts[node.Labels["topology.kubernetes.io/zone"]] += nodeCounts[node.Name]
			}
		}
		for rack, count := range rackCounts {
			Expect(count).To(Equal(expectDaemonCountInRack), "rack=%s, count=%d", rack, count)
		}
	})
}

func testMONPodsSpreadAll() {
	for _, namespace := range []string{"ceph-hdd", "ceph-ssd", "ceph-poc"} {
		testDaemonPodsSpread("MON", "app=rook-ceph-mon", namespace, 3, 1, 1)
	}
}

func testMGRPodsSpreadAll() {
	for _, namespace := range []string{"ceph-hdd", "ceph-ssd", "ceph-poc"} {
		testDaemonPodsSpread("MGR", "app=rook-ceph-mgr", namespace, 2, 1, 1)
	}
}

func testOSDPodsSpread() {
	cephClusterName := "ceph-hdd"
	cephClusterNamespace := "ceph-hdd"
	nodeRole := "ss"

	By("checking OSD Pods for "+cephClusterName+" are spread on "+nodeRole+" nodes", func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", "node-role.kubernetes.io/"+nodeRole+"=true", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		nodes := new(corev1.NodeList)
		err = json.Unmarshal(stdout, nodes)
		Expect(err).ShouldNot(HaveOccurred())

		nodeCounts := make(map[string]int)
		for _, node := range nodes.Items {
			nodeCounts[node.Name] = 0
		}

		stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+cephClusterNamespace,
			"get", "pod", "-l", "app=rook-ceph-osd", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		pods := new(corev1.PodList)
		err = json.Unmarshal(stdout, pods)
		Expect(err).ShouldNot(HaveOccurred())

		for _, pod := range pods.Items {
			nodeCounts[pod.Spec.NodeName]++
		}

		var min int = math.MaxInt32
		var max int
		for _, v := range nodeCounts {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		Expect(max-min).Should(BeNumerically("<=", 1), "nodeCounts=%v", nodeCounts)

		rackCounts := make(map[string]int)
		for _, node := range nodes.Items {
			rackCounts[node.Labels["topology.kubernetes.io/zone"]] += nodeCounts[node.Name]
		}

		min = math.MaxInt32
		max = 0
		for _, v := range rackCounts {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		Expect(max-min).Should(BeNumerically("<=", 1), "rackCounts=%v", rackCounts)
	})
}

func testRookRGW() {
	By("putting/getting data with s3 client", func() {
		ns := "dctest"
		waitRGW(ns, "pod-ob")

		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo 'putting getting data' > /tmp/put_get"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/put_get --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		stdout, _, _ = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd ls s3://\${BUCKET_NAME} --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(stdout).Should(ContainSubstring("put_get"))

		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/put_get /tmp/put_get_download --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "cat", "/tmp/put_get_download")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("putting getting data\n")))
	})
}

func testRookRBDAll() {
	testRookRBD("ceph-hdd-block")
	testRookRBD("ceph-ssd-block")
	testRookRBD("ceph-poc-block")
}

func testRookRBD(storageClassName string) {
	pod := storageClassName + "-pod-rbd"
	By("mounting RBD of "+storageClassName, func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "mountpoint", "-d", "/test1")
			if err != nil {
				return fmt.Errorf("failed to check mount point. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		writePath := "/test1/test.txt"
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "cp", "/etc/passwd", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "sync")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", pod, "--", "cat", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func prepareRebootRookCeph() {
	Context("preparing rook-ceph for reboot", prepareRookCeph)

	It("should store data via RGW before reboot", func() {
		ns := "dctest"
		waitRGW(ns, "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo 'reboot data' > /tmp/reboot"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/reboot --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}/reboot"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

//go:embed testdata/storage-reboot.yaml
var storageRebootYAML []byte

func testRebootRookCeph() {
	It("should get stored data via RGW after reboot", func() {
		By("recreating Pod using OBC")
		_, stderr, err := ExecAtWithInput(boot0, storageRebootYAML, "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		waitRGW("dctest", "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", "dctest", "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/reboot /tmp/reboot_download --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", "dctest", "pod-ob", "--", "cat", "/tmp/reboot_download")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("reboot data\n")))
	})
}

func waitRGW(ns, podName string) {
	Eventually(func() error {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, podName, "--", "sh", "-c",
			`"s3cmd ls s3://\${BUCKET_NAME}/ --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		if err != nil {
			return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
		}
		return nil
	}).Should(Succeed())
}

func testRookCeph() {
	It("should be available", func() {
		testRookOperator()
		testClusterStable()
		testOSDPodsSpread()
		testMONPodsSpreadAll()
		testMGRPodsSpreadAll()
		testRookRGW()
		testRookRBDAll()
	})
}
