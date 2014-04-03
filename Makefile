
all: build

deps:
	go get -u github.com/ugorji/go/codec
	go get -u github.com/cocaine/cocaine-framework-go/cocaine

build: deps
	cd cmd/gofetcher && go build -o ../../app/gofetcher/gofetcher
