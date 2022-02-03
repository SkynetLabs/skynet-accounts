# syntax=docker/dockerfile:1

# Import golang image
FROM golang:1.16-alpine

# Define working directory
WORKDIR /app

# Copy in the go.mod and go.sum files and download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Download the custom dependencies for the generator script. These aren't in
# go.mod because the main_dockerfile isn't tracked by go.mod
#
# This is a workaround for avoiding compile errors for accounts
RUN go get github.com/ory/hydra/jwk

# Copy in the main_dockerfile as main.go
COPY ./lib/main_dockerfile ./main.go

# Build the go program and name the output binary file
RUN go build -o /genenv

# Run the binary
CMD [ "/genenv" ]

