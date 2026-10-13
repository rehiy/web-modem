#!/bin/sh
#

set -e

###########################################

export CGO_ENABLED=0
export GO111MODULE=on
export GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}

UPDATE_REPO=${UPDATE_REPO:-rehiy/web-modem}

build_webui() {
    if ! command -v corepack >/dev/null 2>&1; then
        echo "corepack is required to build webui" >&2
        exit 1
    fi

    (cd webui && corepack pnpm install --frozen-lockfile && corepack pnpm build)
}

build() {
    echo building for $1/$2
    target=build/web-modem-$1-$2
    if [ x"$1" = x"windows" ]; then
        target="${target}.exe"
    fi
    if [ x"$2" = x"arm" ]; then
        export GOARM=7
    fi
    GOOS=$1 GOARCH=$2 go build -ldflags="-s -w -X github.com/rehiy/web-modem/appinfo.Version=$last_tag -X github.com/rehiy/web-modem/appinfo.UpdateRepo=$UPDATE_REPO" -o $target main.go
}

####################################################################

RUN_NUMBER=${GITHUB_RUN_NUMBER:-0}

last_tag=$(git describe --tags --abbrev=0 HEAD 2>/dev/null || echo v0.0.0)
prev_tag=$(git describe --tags --abbrev=0 "$last_tag^" 2>/dev/null || true)
git log "${prev_tag:+$prev_tag..}HEAD" --pretty=format:"%s" | sed '/^release/d; s/^/- /' | sort > RELEASE.md

echo "build info - tag: $last_tag, build: $RUN_NUMBER"

####################################################################

if [ x"${SKIP_WEBUI_BUILD:-0}" = x"1" ]; then
    if [ ! -f webview/dist/index.html ]; then
        echo "webview/dist/index.html is missing; run webui build first" >&2
        exit 1
    fi
else
    build_webui
fi

build linux amd64
build linux arm64
build linux arm
build linux ppc64le
build linux s390x

build darwin amd64
build darwin arm64

build windows amd64
build windows arm64

####################################################################

for app in build/web-modem-*; do
    case "$app" in *.gz) continue ;; esac
    gzip -f "$app"
done

if command -v sha256sum >/dev/null 2>&1; then
    (cd build && sha256sum *.gz > checksums.txt)
else
    (cd build && shasum -a 256 *.gz > checksums.txt)
fi
