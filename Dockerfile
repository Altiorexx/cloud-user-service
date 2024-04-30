FROM ubuntu:latest
FROM golang:1.21
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o build/user_service .
EXPOSE $PORT
CMD ./build/user_service


