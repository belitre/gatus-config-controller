# gatus-config-controller

A Kubernetes controller that watches `Ingress` resources and automatically generates [Gatus](https://github.com/TwiN/gatus) endpoint configuration from the hosts defined in those resources.

## Overview

`gatus-config-controller` runs in your Kubernetes cluster and monitors `Ingress` resources. When it finds hosts, it writes the corresponding Gatus endpoint config to a shared ConfigMap. Gatus reads that ConfigMap and monitors those hosts automatically — no manual config updates needed when services are added, removed, or changed.

## Supported Resources

| Resource | API Group | Status |
|---|---|---|
| `Ingress` | `networking.k8s.io/v1` | Supported |
| `HTTPRoute` | `gateway.networking.k8s.io/v1` | Planned |

## How It Works

1. The controller lists and watches all `Ingress` resources across the cluster.
2. It applies the configured ingress selectors to decide which ingresses to monitor.
3. For each selected ingress host it generates Gatus endpoint entries using the configured check templates.
4. The generated config is written to the `dynamic.yaml` key in a shared ConfigMap.
5. Gatus reads the ConfigMap directory and merges `dynamic.yaml` with any static config.

## Configuration

### Ingress Selection

Ingress selection is controlled by `config.ingresses`. The behaviour depends on whether the key is set and what value it has:

| `config.ingresses` value | Behaviour |
|---|---|
| Key absent (or no `config` at all) | **All ingresses** are selected |
| `ingresses: []` (empty list) | **No ingresses** are selected |
| `ingresses: [...]` (one or more selectors) | Only ingresses matching **at least one** selector are selected |

Selectors are **ORed**: an ingress is included if it matches any selector.
Within a selector, all filters are **ANDed**: an ingress must satisfy every filter in that selector.

#### Available Filters

```yaml
config:
  ingresses:
    - namespaces:
        include: [default, production]   # must be in this list
        exclude: [kube-system]           # must not be in this list
      ingressClasses:
        include: [nginx]
      labels:
        include:
          - key: env
            value: prod                  # all include pairs must be present
        exclude:
          - key: visibility
            value: internal              # none of the exclude pairs may match
      annotations:
        include: [...]
        exclude: [...]
```

#### Examples

```yaml
# Select all ingresses in the default namespace OR with class nginx (except internal ones)
config:
  ingresses:
    - namespaces:
        include: [default]
    - ingressClasses:
        include: [nginx]
      annotations:
        exclude:
          - key: visibility
            value: internal
```

```yaml
# Select no ingresses (monitoring disabled)
config:
  ingresses: []
```

```yaml
# Select all ingresses (omit the ingresses key)
config: {}
```

### Check Templates

By default (no `config` provided) the controller generates a single HTTPS check per host:

```yaml
url: https://<host>
interval: 60s
conditions:
  - "[STATUS] == 200"
```

You can override this globally with `config.defaultChecks`, or per-selector with `checks` on an individual selector. Priority order: **selector checks > defaultChecks > builtin**.

Each check template produces one Gatus endpoint per host:

```yaml
config:
  defaultChecks:
    - nameSuffix: http-redirect      # endpoint name becomes "namespace/name - http-redirect"
      scheme: http                   # "http" or "https"
      interval: 60s
      noFollowRedirects: true        # sets client.ignore-redirect: true in Gatus
      conditions:
        - "[STATUS] == 301"

    - nameSuffix: https
      scheme: https
      interval: 60s
      conditions:
        - "[STATUS] == 200"

    - nameSuffix: oauth2-redirect
      scheme: https
      interval: 60s
      noFollowRedirects: true
      conditions:
        - "[STATUS] == 302"
        - "[RESPONSE_HEADER.Location] == *auth.example.com*"   # glob match

  ingresses:
    - ingressClasses:
        include: [nginx]
      # This selector uses a tighter check, overriding defaultChecks
      checks:
        - nameSuffix: https-strict
          scheme: https
          interval: 30s
          conditions:
            - "[STATUS] == 200"
            - "[RESPONSE_TIME] < 500"
```

When `nameSuffix` is empty, the endpoint name is just `namespace/name`. If the same check content would be generated twice (e.g. the same ingress matched by two selectors with identical checks), duplicates are suppressed.

### Static Config

Static Gatus endpoints (ones not derived from ingresses) can be provided via `staticConfig` in the Helm values. They are written to `static.yaml` in the same ConfigMap and merged by Gatus alongside the dynamic endpoints.

```yaml
staticConfig:
  endpoints:
    - name: example
      url: https://example.org
      interval: 60s
      conditions:
        - "[STATUS] == 200"
```

## Helm Chart

The chart deploys the controller and Gatus as a subchart. Gatus is pre-configured to read its config from the shared ConfigMap.

### Key Values

| Value | Default | Description |
|---|---|---|
| `image.repository` | `gatus-config-controller` | Controller image |
| `image.tag` | `canary` | Image tag |
| `configMapNamespace` | `default` | Namespace of the shared ConfigMap |
| `configMapName` | `gatus-dynamic-config` | Name of the shared ConfigMap |
| `logLevel` | `""` (info) | Log level: `debug`, `info`, `error` |
| `config` | `{}` | Controller config (ingress selection + checks) |
| `staticConfig` | `{}` | Static Gatus endpoints |
| `gatus.*` | see values.yaml | Gatus subchart values |

## Local Development

### Prerequisites

- [kind](https://kind.sigs.k8s.io/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [helm](https://helm.sh/)
- [Docker](https://www.docker.com/)

### Setup

Add `gatus.local` to your `/etc/hosts`:

```
127.0.0.1 gatus.local
```

Spin up the cluster with ingress-nginx and Gatus:

```bash
make env-up
```

Gatus will be available at `http://gatus.local`.

To tear down:

```bash
make env-down
```

Local environment values live in [hack/gatus-config-controller-values.yaml](hack/gatus-config-controller-values.yaml).

### Makefile Targets

| Target | Description |
|---|---|
| `make env-up` | Create kind cluster, deploy ingress-nginx, build and deploy the controller + Gatus |
| `make env-down` | Delete the kind cluster |
| `make build` | Build the controller binary locally |
| `make docker-build` | Build the controller Docker image |
| `make run` | Run the controller locally with `go run` |
| `make preload-images` | Pre-pull and load images into kind (set `PRELOAD_IMAGES` to override the list) |

## License

Apache License 2.0 — see [LICENSE](LICENSE).
