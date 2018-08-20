FROM alpine:edge
RUN apk add --no-cache ca-certificates
COPY adstxt /adstxt
CMD ["/adstxt"]
