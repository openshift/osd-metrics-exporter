FROM registry.svc.ci.openshift.org/openshift/release:golang-1.13 AS builder
COPY go.mod /
COPY go.sum /
RUN go mod download
COPY . /osd-metrics-exporter
WORKDIR /osd-metrics-exporter
ENV GO111MODULE=on
ENV GOFLAGS='-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"'
RUN go build -o build/_output/bin/osd-metrics-exporter ./cmd/manager/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV OPERATOR=/usr/local/bin/osd-metrics-exporter \
    USER_UID=1001 \
    USER_NAME=osd-metrics-exporter
# install operator binary
COPY --from=builder /osd-metrics-exporter/build/_output/bin/osd-metrics-exporter ${OPERATOR}
COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup
ENTRYPOINT ["/usr/local/bin/entrypoint"]
USER ${USER_UID}
