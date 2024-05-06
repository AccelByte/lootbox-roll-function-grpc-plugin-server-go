# Copyright (c) 2022 AccelByte Inc. All Rights Reserved.
# This is licensed software from AccelByte Inc, for limitations
# and restrictions contact your company contract manager.

SHELL := /bin/bash

IMAGE_NAME := $(shell basename "$$(pwd)")-app
BUILDER := extend-builder

GOLANG_DOCKER_IMAGE := golang:1.19

TEST_SAMPLE_CONTAINER_NAME := sample-override-test

proto:
	rm -rfv pkg/pb/*
	mkdir -p pkg/pb
	docker run -t --rm -u $$(id -u):$$(id -g) -v $$(pwd):/data/ -w /data/ rvolosatovs/protoc:4.0.0 \
			--proto_path=pkg/proto --go_out=pkg/pb \
			--go_opt=paths=source_relative --go-grpc_out=pkg/pb \
			--go-grpc_opt=paths=source_relative pkg/proto/*.proto

lint:
	rm -f lint.err
	find -type f -iname go.mod -not -path "*/.cache/*" -exec dirname {} \; | while read DIRECTORY; do \
		echo "# $$DIRECTORY"; \
		docker run -t --rm -u $$(id -u):$$(id -g) -v $$(pwd):/data/ -w /data/ -e GOCACHE=/data/.cache/go-build -e GOLANGCI_LINT_CACHE=/data/.cache/go-lint golangci/golangci-lint:v1.52.2\
				sh -c "cd $$DIRECTORY && golangci-lint -v --timeout 5m --max-same-issues 0 --max-issues-per-linter 0 --color never run || touch /data/lint.err"; \
	done
	[ ! -f lint.err ] || (rm lint.err && exit 1)

build: proto
	docker run -t --rm -u $$(id -u):$$(id -g) -v $$(pwd):/data/ -w /data/ -e GOCACHE=/data/.cache/go-build $(GOLANG_DOCKER_IMAGE) \
		sh -c "go build"

image:
	docker buildx build -t ${IMAGE_NAME} --load .

imagex:
	docker buildx inspect $(BUILDER) || docker buildx create --name $(BUILDER) --use
	docker buildx build -t ${IMAGE_NAME} --platform linux/amd64 .
	docker buildx build -t ${IMAGE_NAME} --load .
	docker buildx rm --keep-state $(BUILDER)

imagex_push:
	@test -n "$(IMAGE_TAG)" || (echo "IMAGE_TAG is not set (e.g. 'v0.1.0', 'latest')"; exit 1)
	@test -n "$(REPO_URL)" || (echo "REPO_URL is not set"; exit 1)
	docker buildx inspect $(BUILDER) || docker buildx create --name $(BUILDER) --use
	docker buildx build -t ${REPO_URL}:${IMAGE_TAG} --platform linux/amd64 --push .
	docker buildx rm --keep-state $(BUILDER)

ngrok:
	@which ngrok || (echo "ngrok is not installed" ; exit 1)
	@test -n "$(NGROK_AUTHTOKEN)" || (echo "NGROK_AUTHTOKEN is not set" ; exit 1)
	ngrok tcp 6565	# gRPC server port

test_sample_local_hosted:
	@test -n "$(ENV_PATH)" || (echo "ENV_PATH is not set"; exit 1)
	docker build \
			--tag $(TEST_SAMPLE_CONTAINER_NAME) \
			-f test/sample/Dockerfile \
			test/sample
	docker run --rm -t \
			-u $$(id -u):$$(id -g) \
			-e GOCACHE=/data/.cache/go-build \
			-e GOPATH=/data/.cache/mod \
			--env-file $(ENV_PATH) \
			-v $$(pwd):/data \
			-w /data \
			--name $(TEST_SAMPLE_CONTAINER_NAME) \
			$(TEST_SAMPLE_CONTAINER_NAME) \
			bash ./test/sample/test-local-hosted.sh

test_sample_accelbyte_hosted:
	@test -n "$(ENV_PATH)" || (echo "ENV_PATH is not set"; exit 1)
ifeq ($(shell uname), Linux)
	$(eval DARGS := -u $$(shell id -u) --group-add $$(shell getent group docker | cut -d ':' -f 3))
endif
	docker build \
			--tag $(TEST_SAMPLE_CONTAINER_NAME) \
			-f test/sample/Dockerfile \
			test/sample
	docker run --rm -t \
			-e GOCACHE=/data/.cache/go-build \
			-e GOPATH=/data/.cache/mod \
			-e DOCKER_CONFIG=/tmp/.docker \
			--env-file $(ENV_PATH) \
			-v /var/run/docker.sock:/var/run/docker.sock \
			-v $$(pwd):/data \
			-w /data \
			--name $(TEST_SAMPLE_CONTAINER_NAME) \
			$(DARGS) \
			$(TEST_SAMPLE_CONTAINER_NAME) \
			bash ./test/sample/test-accelbyte-hosted.sh
