package test

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func prepareLoadPods() {
	It("should deploy pods", func() {
		yamlCS := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: addload-for-cs
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: addload
  template:
    metadata:
      labels:
        app: addload
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - addload
            topologyKey: "kubernetes.io/hostname"
      containers:
      - name: spread-test-ubuntu
        image: quay.io/cybozu/ubuntu:20.04
        command:
        - "/usr/local/bin/pause"
        securityContext:
          runAsUser: 10000
          runAsGroup: 10000
        resources:
          requests:
            cpu: "2"
`
		stdout, stderr, err := ExecAtWithInput(boot0, []byte(yamlCS), "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		yamlSS := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: addload-for-ss
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: addload
  template:
    metadata:
      labels:
        app: addload
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - addload
            topologyKey: "kubernetes.io/hostname"
      containers:
      - name: spread-test-ubuntu
        image: quay.io/cybozu/ubuntu:20.04
        command:
        - "/usr/local/bin/pause"
        securityContext:
          runAsUser: 10000
          runAsGroup: 10000
        resources:
          requests:
            cpu: "1"
      nodeSelector:
        cke.cybozu.com/role: ss
      tolerations:
      - key: cke.cybozu.com/role
        operator: Equal
        value: storage
`
		stdout, stderr, err = ExecAtWithInput(boot0, []byte(yamlSS), "kubectl", "apply", "-f", "-")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl",
				"get", "deployment", "addload-for-cs", "-o=json")
			if err != nil {
				return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}

			deployment := new(appsv1.Deployment)
			err = json.Unmarshal(stdout, deployment)
			if err != nil {
				return fmt.Errorf("stdout: %s, err: %v", stdout, err)
			}

			if deployment.Status.AvailableReplicas != 2 {
				return fmt.Errorf("addload-for-cs deployment's AvailableReplicas is not 2: %d", int(deployment.Status.AvailableReplicas))
			}

			return nil
		}).Should(Succeed())

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

func prepareRookCeph() {
	It("should create test-rook-rgw namespace for testRookRGW", func() {
		ExecSafeAt(boot0, "kubectl", "delete", "namespace", "test-rook-rgw", "--ignore-not-found=true")
		createNamespaceIfNotExists("test-rook-rgw")
		ExecSafeAt(boot0, "kubectl", "annotate", "namespaces", "test-rook-rgw", "i-am-sure-to-delete=test-rook-rgw")
	})

	It("should apply a OBC resource and a POD for testRookRGW", func() {
		ns := "test-rook-rgw"
		podPvcYaml := fmt.Sprintf(`apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: pod-ob
  namespace: %s
spec:
  generateBucketName: obc-poc
  storageClassName: ceph-hdd-bucket
---
apiVersion: v1
kind: Pod
metadata:
  name: pod-ob
  namespace: %s
spec:
  containers:
  - name: mycontainer
    image: quay.io/cybozu/ubuntu-debug:20.04
    imagePullPolicy: Always
    args:
    - infinity
    command:
    - sleep
    envFrom:
    - configMapRef:
        name: pod-ob
    - secretRef:
        name: pod-ob`, ns, ns)

		_, stderr, err := ExecAtWithInput(boot0, []byte(podPvcYaml), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
	})

	for _, storageClassName := range []string{"ceph-hdd-block", "ceph-ssd-block"} {
		ns := "test-rook-rbd-" + storageClassName
		It("should create "+ns+" namespace for testRookRBD", func() {
			ExecSafeAt(boot0, "kubectl", "delete", "namespace", ns, "--ignore-not-found=true")
			createNamespaceIfNotExists(ns)
			ExecSafeAt(boot0, "kubectl", "annotate", "namespaces", ns, "i-am-sure-to-delete="+ns)
		})

		It("should create a POD for testRookRBD", func() {
			podPvcYaml := fmt.Sprintf(`kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: pvc-rbd
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: %s
---
apiVersion: v1
kind: Pod
metadata:
  name: pod-rbd
  labels:
    app.kubernetes.io/name: pod-rbd
spec:
  containers:
  - name: ubuntu
    image: quay.io/cybozu/ubuntu-debug:20.04
    imagePullPolicy: Always
    command: ["/usr/local/bin/pause"]
    volumeMounts:
    - mountPath: /test1
      name: rbd-volume
  volumes:
  - name: rbd-volume
    persistentVolumeClaim:
      claimName: pvc-rbd`, storageClassName)

			_, stderr, err := ExecAtWithInput(boot0, []byte(podPvcYaml), "kubectl", "apply", "-n", ns, "-f", "-")
			Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		})
	}
}

func testRookOperator() {
	nss := []string{"ceph-hdd", "ceph-ssd"}
	for _, ns := range nss {
		It("should deploy rook operator to "+ns+" ns successfully", func() {
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

		It("should deploy ceph tools to "+ns+" correctly", func() {
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
	nss := []string{"ceph-hdd", "ceph-ssd"}
	for _, ns := range nss {
		It("should be rook/ceph cluster("+ns+") stable", func() {
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
				"get", "cephcluster", ns, "-o", "jsonpath='{.spec.storage.storageClassDeviceSets[0].count}'")
			Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
			num_osd_expected, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
			Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)

			num_rgw_expected := 0
			if ns == "ceph-hdd" {
				stdout, stderr, err := ExecAt(boot0, "kubectl", "--namespace="+ns,
					"get", "cephobjectstore", ns+"-object-store", "-o", "jsonpath='{.spec.gateway.instances}'")
				Expect(err).ShouldNot(HaveOccurred(), "stderr=%s", stderr)
				n, err := strconv.Atoi(strings.TrimSpace(string(stdout)))
				Expect(err).ShouldNot(HaveOccurred(), "stdout=%s", stdout)
				num_rgw_expected = n
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

func testMONPodsSpreadAll() {
	testMONPodsSpread("ceph-hdd", "ceph-hdd")
	testMONPodsSpread("ceph-ssd", "ceph-ssd")
}

func testMONPodsSpread(cephClusterName, cephClusterNamespace string) {
	It("should spread MON PODs", func() {
		stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "node", "-l", "node-role.kubernetes.io/cs=true", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		nodes := new(corev1.NodeList)
		err = json.Unmarshal(stdout, nodes)
		Expect(err).ShouldNot(HaveOccurred())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "--namespace="+cephClusterNamespace,
			"get", "pod", "-l", "app=rook-ceph-mon", "-o=json")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		pods := new(corev1.PodList)
		err = json.Unmarshal(stdout, pods)
		Expect(err).ShouldNot(HaveOccurred())

		nodeCounts := make(map[string]int)
		for _, pod := range pods.Items {
			nodeCounts[pod.Spec.NodeName]++
		}
		for node, count := range nodeCounts {
			Expect(count).To(Equal(1), "node=%s, count=%d", node, count)
		}
		Expect(nodeCounts).Should(HaveLen(3))

		rackCounts := make(map[string]int)
		for _, node := range nodes.Items {
			rackCounts[node.Labels["topology.kubernetes.io/zone"]] += nodeCounts[node.Name]
		}
		for node, count := range nodeCounts {
			Expect(count).To(Equal(1), "node=%s, count=%d", node, count)
		}
		Expect(nodeCounts).Should(HaveLen(3))
	})
}

func testOSDPodsSpreadAll() {
	testOSDPodsSpread("ceph-hdd", "ceph-hdd", "ss")
	testOSDPodsSpread("ceph-ssd", "ceph-ssd", "cs")
}

func testOSDPodsSpread(cephClusterName, cephClusterNamespace, nodeRole string) {
	It("should spread OSD PODs on "+nodeRole+" nodes", func() {
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
	It("should be used from a POD with a s3 client", func() {
		ns := "test-rook-rgw"
		waitRGW(ns, "pod-ob")

		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo foobar > /tmp/foobar"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/foobar --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)

		stdout, _, _ = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd ls s3://\${BUCKET_NAME} --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}"`)
		Expect(stdout).NotTo(BeEmpty())

		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/foobar /tmp/downloaded --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "cat", "/tmp/downloaded")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("foobar\n")))
	})
}

func testRookRBDAll() {
	testRookRBD("ceph-hdd-block")
	testRookRBD("ceph-ssd-block")
}

func testRookRBD(storageClassName string) {
	ns := "test-rook-rbd-" + storageClassName
	It("should be mounted to a path specified on a POD", func() {
		Eventually(func() error {
			stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-rbd", "--", "mountpoint", "-d", "/test1")
			if err != nil {
				return fmt.Errorf("failed to check mount point. stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
			}
			return nil
		}).Should(Succeed())

		writePath := "/test1/test.txt"
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-rbd", "--", "cp", "/etc/passwd", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-rbd", "--", "sync")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-rbd", "--", "cat", writePath)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func prepareRebootRookCeph() {
	Context("preparing rook-ceph for reboot", prepareRookCeph)

	It("should store data via RGW before reboot", func() {
		ns := "test-rook-rgw"
		waitRGW(ns, "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c", `"echo foobar > /tmp/foobar"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd put /tmp/foobar --no-ssl --host=\${BUCKET_HOST} --host-bucket= s3://\${BUCKET_NAME}/foobar_reboot"`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
	})
}

func testRebootRookCeph() {
	It("should get stored data via RGW after reboot", func() {
		ns := "test-rook-rgw"
		By("recreating Pod using OBC")
		podPvcYaml := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: pod-ob
  namespace: %s
spec:
  containers:
  - name: mycontainer
    image: quay.io/cybozu/ubuntu-debug:20.04
    imagePullPolicy: Always
    args:
    - infinity
    command:
    - sleep
    envFrom:
    - configMapRef:
        name: pod-ob
    - secretRef:
        name: pod-ob`, ns)
		_, stderr, err := ExecAtWithInput(boot0, []byte(podPvcYaml), "kubectl", "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		waitRGW(ns, "pod-ob")
		stdout, stderr, err := ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "sh", "-c",
			`"s3cmd get s3://\${BUCKET_NAME}/foobar_reboot /tmp/downloaded --no-ssl --host=\${BUCKET_HOST} --host-bucket="`)
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		stdout, stderr, err = ExecAt(boot0, "kubectl", "exec", "-n", ns, "pod-ob", "--", "cat", "/tmp/downloaded")
		Expect(err).ShouldNot(HaveOccurred(), "stdout=%s, stderr=%s", stdout, stderr)
		Expect(stdout).To(Equal([]byte("foobar\n")))
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
	Context("rookOperator", testRookOperator)
	Context("clusterStable", testClusterStable)
	Context("OSDPodsSpread", testOSDPodsSpreadAll)
	Context("MONPodsSpread", testMONPodsSpreadAll)
	Context("rookRGW", testRookRGW)
	Context("rookRBD", testRookRBDAll)
}
