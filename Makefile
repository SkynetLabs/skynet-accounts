# These variables get inserted into ./build/commit.go
BUILD_TIME=$(shell date)
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")

ldflags= -X github.com/NebulousLabs/skynet-accounts/build.GitRevision=${GIT_DIRTY}${GIT_REVISION} \
-X "github.com/NebulousLabs/skynet-accounts/build.BuildTime=${BUILD_TIME}"

racevars= history_size=3 halt_on_error=1 atexit_sleep_ms=2000

# all will build and install release binaries
all: release

# count says how many times to run the tests.
count = 1
# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
pkgs = ./api ./build ./db ./user

# lockcheckpkgs are the packages that are checked for locking violations.
lockcheckpkgs = ./api ./build ./db ./user

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet:
	go vet $(pkgs)

# markdown-spellcheck runs codespell on all markdown files that are not
# vendored.
markdown-spellcheck:
	git ls-files "*.md" :\!:"vendor/**" | xargs codespell --check-filenames

# lint runs golangci-lint (which includes golint, a spellcheck of the codebase,
# and other linters), the custom analyzers, and also a markdown spellchecker.
lint: markdown-spellcheck lint-analyze
	golangci-lint run -c .golangci.yml

# lint-ci runs golint.
lint-ci:
# golint is skipped on Windows.
ifneq ("$(OS)","Windows_NT")
# Linux
	go get golang.org/x/lint/golint
	golint -min_confidence=1.0 -set_exit_status $(pkgs)
endif

# lint-analyze runs the custom analyzers.
lint-analyze:
	analyze -lockcheck=false -- $(pkgs)
	analyze -lockcheck -- $(lockcheckpkgs)

# spellcheck checks for misspelled words in comments or strings.
spellcheck: markdown-spellcheck
	golangci-lint run -c .golangci.yml -E misspell

# staticcheck runs the staticcheck tool
# NOTE: this is not yet enabled in the CI system.
staticcheck:
	staticcheck $(pkgs)

# debug builds and installs debug binaries. This will also install the utils.
debug:
	go install -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
debug-race:
	GORACE='$(racevars)' go install -race -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# dev builds and installs developer binaries. This will also install the utils.
dev:
	go install -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
dev-race:
	GORACE='$(racevars)' go install -race -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-race:
	GORACE='$(racevars)' go install -race -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-util:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs) $(util-pkgs)


test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=$(run) -count=$(count)
test-v:
	GORACE='$(racevars)' go test -race -v -short -tags='debug testing netgo' -timeout=15s $(pkgs) -run=$(run) -count=$(count)
test-long: clean fmt vet lint-ci
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -failfast -tags='testing debug netgo' -timeout=3600s $(pkgs) -run=$(run) -count=$(count)

.PHONY: all fmt install release clean test test-v test-long
