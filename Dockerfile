FROM golang:rc as build
WORKDIR /build
COPY . /build
RUN go build -o adstxt ./cmd

FROM gcr.io/distroless/base
COPY --from=build /build/adstxt /adstxt
CMD ["/adstxt"]
