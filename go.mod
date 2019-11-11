module github.com/cybozu-go/neco-apps

go 1.13

replace (
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190805141520-2fe0317bcee0
	launchpad.net/gocheck => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
)

require (
	github.com/argoproj/argo-cd v1.1.0-rc7
	github.com/argoproj/pkg v0.0.0-20190708182346-fb13aebbef1c // indirect
	github.com/creack/pty v1.1.7
	github.com/cybozu-go/log v1.5.0
	github.com/cybozu-go/sabakan/v2 v2.4.2
	github.com/google/go-cmp v0.3.0
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/jetstack/cert-manager v0.11.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/onsi/ginkgo v1.10.2
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/common v0.7.0
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	google.golang.org/appengine v1.6.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.11.0 // indirect
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	sigs.k8s.io/yaml v1.1.0
)
