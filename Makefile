.PHONY: all
all: apollo-confd

.PHONY: apollo-confd
apollo-confd:
	go build github.com/gofly/apollo-confd

.PHONY: linux
linux:
	mkdir -p apollo-confd_linxu_x86_64 && \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -o apollo-confd_linxu_x86_64/apollo-confd github.com/gofly/apollo-confd && \
	cp apollo-confd.yaml apollo-confd_linxu_x86_64