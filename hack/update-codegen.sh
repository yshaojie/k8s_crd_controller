#!/usr/bin/env bash
set -o errexit
set -o nounset
#set -o pipefail
GO111MODULE=on
export GOPROXY=https://proxy.golang.com.cn,direct
export GOPATH=/home/shaojieyue/go
#下载依赖,依赖不全会导致不能生成informers和listers
go mod download
go mod tidy
go get -u -v k8s.io/code-generator

#如生成代码有问题，可以生成部分代码来进行调试，如将all替换为informers
bash $GOPATH/pkg/mod/k8s.io/code-generator@v0.24.3/generate-groups.sh all \
k8s_crd_controller/pkg/generated \
k8s_crd_controller/pkg/apis \
crd:v1 \
--output-base ../  --go-header-file hack/boilerplate.go.txt -v 10