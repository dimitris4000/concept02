# Build Stage
# First pull Golang image
FROM golang:1.22-bookworm as build-env
 
# Set environment variable
ENV APP_NAME concept02
ENV CMD_PATH main.go
 
# Copy application data into image
COPY . $GOPATH/src/$APP_NAME
WORKDIR $GOPATH/src/$APP_NAME
 
# Budild application
RUN CGO_ENABLED=0 go build -v -o /$APP_NAME $GOPATH/src/$APP_NAME/$CMD_PATH


# Run Stage
FROM scratch
 
# Set environment variable
ENV APP_NAME concept02
 
# Copy only required data into this image
COPY --from=build-env /$APP_NAME /bin/concept02
 
# Expose application port
EXPOSE 8081
 
# Configure entrypoint
ENTRYPOINT ["/bin/concept02"]

