FROM golang:alpine AS build
ENV APP_NAME manager-service
ENV PORT 10000
COPY . /go/src/manager-service
WORKDIR /go/src/manager-service
RUN CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o /go/bin/manager-service -tags timetzdata
FROM scratch
COPY --from=build /go/bin/manager-service /go/bin/manager-service
CMD [ "/go/bin/manager-service" ]
