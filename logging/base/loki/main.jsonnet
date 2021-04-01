local loki = import 'loki/loki.libsonnet';

loki {
  _config+:: {
    namespace: 'logging',

    // S3 variables remove if not using aws
    storage_backend: 's3',
    s3_access_key: '${AWS_ACCESS_KEY_ID}',
    s3_secret_access_key: '${AWS_SECRET_ACCESS_KEY}',
    s3_address: '${BUCKET_HOST}',
    s3_bucket_name: '${BUCKET_NAME}',
    s3_path_style: true,

    boltdb_shipper_shared_store: 's3',

    ingester_pvc_class: 'ceph-ssd-block',
    querier_pvc_class: 'ceph-ssd-block',
    compactor_pvc_class: 'ceph-ssd-block',

    commonArgs+: {
      'config.expand-env': 'true',
    },

    replication_factor: 3,

    loki+: {
      auth_enabled: false,

      ingester+: {
        lifecycler+: {
          ring+: {
            kvstore+: {
              consul+: {
                host: 'logging-consul-server.logging.svc:8500'
              },
            },
          },
        },
      },

      distributor+: {
        ring+: {
          kvstore+: {
            consul+: {
              host: 'logging-consul-server.logging.svc:8500'
            },
          },
        },
      },

      schema_config+: {
        configs: [
          x {
            object_store: 's3',
            index+: {
              prefix: 'index_'
            },
          }
          for x in super.configs
        ],
      },
    },
  },

  _images+:: {
    memcached: 'quay.io/cybozu/memcached',
    memcachedExporter: 'quay.io/cybozu/memcached-exporter',
    loki: 'quay.io/cybozu/loki'
  },

  compactor_args+:: {
    'config.expand-env': 'true',
  },
}
