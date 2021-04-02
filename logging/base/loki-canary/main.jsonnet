local loki_canary = import 'loki-canary/loki-canary.libsonnet';

loki_canary {
  loki_canary_args+:: {
    addr: "querier.logging.svc:3100",
    labelname: "pod",
    size: 1024,
    wait: "3m",
  },
  _config+:: {
    namespace: "logging",
  },

  _images+:: {
    loki_canary: 'quay.io/cybozu/loki'
  },
}
