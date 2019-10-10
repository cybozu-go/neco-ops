module github.com/cybozu-go/neco-apps

go 1.12

replace launchpad.net/gocheck => github.com/go-check/check v0.0.0-20180628173108-788fd7840127

require (
	github.com/argoproj/argo-cd v1.1.0-rc7
	github.com/argoproj/pkg v0.0.0-20190708182346-fb13aebbef1c // indirect
	github.com/creack/pty v1.1.7
	github.com/cybozu-go/log v1.5.0
	github.com/cybozu-go/sabakan/v2 v2.4.2
	github.com/google/go-cmp v0.3.0
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/jetstack/cert-manager v0.8.0
	github.com/json-iterator/go v1.1.6 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/onsi/ginkgo v1.10.2
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_golang v0.9.3
	github.com/prometheus/common v0.4.1
	github.com/prometheus/tsdb v0.8.0 // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	golang.org/x/crypto v0.0.0-20190530122614-20be4c3c3ed5
	golang.org/x/oauth2 v0.0.0-20190523182746-aaccbc9213b0 // indirect
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190530182044-ad28b68e88f1 // indirect
	google.golang.org/appengine v1.6.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.11.0 // indirect
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190819141258-3544db3b9e44
	k8s.io/apiextensions-apiserver v0.0.0-20190528080216-be227facef18
	k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/client-go v0.0.0-20190819141724-e14f31a72a77 // indirect
	k8s.io/klog v0.3.2 // indirect
	k8s.io/kube-openapi v0.0.0-20190401085232-94e1e7b7574c // indirect
	k8s.io/utils v0.0.0-20190529001817-6999998975a7 // indirect
	sigs.k8s.io/yaml v1.1.0
)
