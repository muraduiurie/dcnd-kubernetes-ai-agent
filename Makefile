# Code generation for the ai.dncp.io API types.

SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS := -ec

LOCALBIN ?= $(CURDIR)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.22.0

CRD_OUTPUT ?= config/crd/bases

.PHONY: all
all: generate manifests

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Install controller-gen into ./bin.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(CONTROLLER_GEN) && $(CONTROLLER_GEN) --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
		GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: generate
generate: controller-gen ## Generate zz_generated.deepcopy.go for the API types.
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: manifests
manifests: controller-gen ## Generate CustomResourceDefinition manifests.
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=$(CRD_OUTPUT)

.PHONY: clean-generated
clean-generated: ## Remove generated deepcopy code and CRD manifests.
	rm -f api/*/zz_generated.deepcopy.go
	rm -rf $(CRD_OUTPUT)

.PHONY: test
test: ## Run all unit tests with the race detector.
	go test -race ./...
