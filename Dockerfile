FROM google/golang

RUN go get github.com/tools/godep

RUN mkdir -p /gopath/src/github.com/lavab/ritratt
ADD . /gopath/src/github.com/lavab/ritratt
RUN cd /gopath/src/github.com/lavab/ritratt && godep go install

ENTRYPOINT ["/gopath/bin/ritratt"]
