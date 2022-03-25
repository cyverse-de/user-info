FROM golang:1.17

WORKDIR /build

COPY . .
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

RUN go install -v ./...

ENTRYPOINT ["user-info"]
CMD ["--help"]
EXPOSE 60000

ARG git_commit=unknown
ARG version="2.9.0"
ARG descriptive_version=unknown

LABEL org.cyverse.git-ref="$git_commit"
LABEL org.cyverse.version="$version"
LABEL org.cyverse.descriptive-version="$descriptive_version"
LABEL org.label-schema.vcs-ref="$git_commit"
LABEL org.label-schema.vcs-url="https://github.com/cyverse-de/user-info"
LABEL org.label-schema.version="$descriptive_version"
