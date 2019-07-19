module github.com/cybozu-go/neco-apps

go 1.12

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go => k8s.io/client-go v11.0.0+incompatible
	launchpad.net/gocheck => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
)

require (
	github.com/argoproj/argo-cd v1.1.0-rc7
	github.com/argoproj/pkg v0.0.0-20190708182346-fb13aebbef1c // indirect
	github.com/cybozu-go/log v1.5.0
	github.com/cybozu-go/sabakan/v2 v2.4.2
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/jetstack/cert-manager v0.8.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kubernetes-incubator/external-dns v0.5.14
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/prometheus/client_golang v0.9.3
	github.com/prometheus/common v0.4.1
	github.com/prometheus/tsdb v0.8.0 // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	golang.org/x/crypto v0.0.0-20190530122614-20be4c3c3ed5
	golang.org/x/net v0.0.0-20190522155817-f3200d17e092 // indirect
	golang.org/x/oauth2 v0.0.0-20190523182746-aaccbc9213b0 // indirect
	golang.org/x/sys v0.0.0-20190530182044-ad28b68e88f1 // indirect
	google.golang.org/appengine v1.6.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.11.0 // indirect
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190528154508-67ef80593b24
	k8s.io/apiextensions-apiserver v0.0.0-20190528080216-be227facef18
	k8s.io/apimachinery v0.0.0-20190528154326-e59c2fb0a8e5
	k8s.io/client-go v11.0.0+incompatible // indirect
	k8s.io/klog v0.3.2 // indirect
	k8s.io/utils v0.0.0-20190529001817-6999998975a7 // indirect
)
