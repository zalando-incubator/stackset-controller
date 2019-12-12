#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SRC="github.com"
GOPKG="$SRC/zalando-incubator/stackset-controller"
CUSTOM_RESOURCE_NAME="zalando.org"
CUSTOM_RESOURCE_VERSION="v1"

SCRIPT_ROOT="$(dirname ${BASH_SOURCE[0]})/.."
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}
OUTPUT_BASE="$(dirname ${BASH_SOURCE})/"

go mod vendor

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash "${CODEGEN_PKG}"/generate-groups.sh all \
  "${GOPKG}/pkg/client" "${GOPKG}/pkg/apis" \
  "${CUSTOM_RESOURCE_NAME}:${CUSTOM_RESOURCE_VERSION}" \
  --go-header-file hack/boilerplate.go.txt \
  --output-base "$OUTPUT_BASE"

# To use your own boilerplate text append:
#   --go-header-file ${SCRIPT_ROOT}/hack/custom-boilerplate.go.txt

# hack to make the generated code work with Go module based projects
cp -r "$OUTPUT_BASE/$GOPKG/pkg/apis" ./pkg
cp -r "$OUTPUT_BASE/$GOPKG/pkg/client" ./pkg
rm -rf "${OUTPUT_BASE:?}${SRC}"
