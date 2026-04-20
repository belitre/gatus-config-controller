CLUSTER_NAME          ?= gatus-config-controller
VERSION               ?= canary
IMAGE                 ?= ghcr.io/belitre/gatus-config-controller
CHART_REGISTRY        ?= ghcr.io/belitre/charts
CHART_VERSION         ?= $(VERSION)
INGRESS_NGINX_MANIFEST := https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

PRELOAD_IMAGES ?= \
	registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1 \
	registry.k8s.io/ingress-nginx/kube-webhook-certgen:v1.6.9@sha256:01038e7de14b78d702d2849c3aad72fd25903c4765af63cf16aa3398f5d5f2dd

.PHONY: build test lint run \
	docker-login docker-build docker-push docker-build-push \
	helm-validate helm-login helm-package helm-push \
	install-semantic-release release release-dry-run \
	env-up env-down preload-images

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(shell git rev-parse --short HEAD) -X main.Date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/gatus-config-controller ./cmd/

# ── Test & Lint ───────────────────────────────────────────────────────────────

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/main.go

# ── Docker ────────────────────────────────────────────────────────────────────

docker-login:
	echo "$(CR_TOKEN)" | docker login ghcr.io -u $(CR_USER) --password-stdin

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(shell git rev-parse --short HEAD) \
		--build-arg DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) \
		-t $(IMAGE):$(VERSION) .

docker-push:
	docker push $(IMAGE):$(VERSION)

docker-build-push: docker-build docker-push

# ── Helm ──────────────────────────────────────────────────────────────────────

helm-validate:
	helm dependency update ./chart/gatus-config-controller
	helm lint ./chart/gatus-config-controller
	helm template test ./chart/gatus-config-controller
	helm template test ./chart/gatus-config-controller --values hack/gatus-config-controller-values.yaml

helm-login:
	echo "$(CR_TOKEN)" | helm registry login ghcr.io -u $(CR_USER) --password-stdin

helm-package:
	helm dependency update ./chart/gatus-config-controller
	helm package ./chart/gatus-config-controller --version $(CHART_VERSION) --app-version $(CHART_VERSION)

helm-push: helm-package
	helm push gatus-config-controller-$(CHART_VERSION).tgz oci://$(CHART_REGISTRY)
	rm -f gatus-config-controller-$(CHART_VERSION).tgz

# ── Semantic Release ──────────────────────────────────────────────────────────

install-semantic-release:
	npm install -g \
		semantic-release@latest \
		@semantic-release/git@latest \
		@semantic-release/changelog@latest \
		@semantic-release/exec@latest \
		conventional-changelog-conventionalcommits@latest

release:
	npx semantic-release

release-dry-run:
	npx semantic-release --dry-run

# ── Local Dev ─────────────────────────────────────────────────────────────────

env-up:
	kind create cluster --name $(CLUSTER_NAME) --config hack/kind-config.yaml
	kubectl apply -f $(INGRESS_NGINX_MANIFEST)
	kubectl rollout status deployment/ingress-nginx-controller -n ingress-nginx --timeout=300s
	$(MAKE) docker-build
	kind load docker-image $(IMAGE):$(VERSION) --name $(CLUSTER_NAME)
	helm dependency update ./chart/gatus-config-controller
	helm install gatus-config-controller ./chart/gatus-config-controller --values hack/gatus-config-controller-values.yaml

preload-images:
	@for img in $(PRELOAD_IMAGES); do \
		docker image inspect $$img > /dev/null 2>&1 || docker pull $$img; \
		kind load docker-image $$img --name $(CLUSTER_NAME); \
	done

env-down:
	kind delete cluster --name $(CLUSTER_NAME)
