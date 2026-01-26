# syntax=docker/dockerfile:1

FROM golang:1.25
FROM --platform=linux/amd64 golang:1.25

WORKDIR /app

# 5. Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# 6. Copy the rest of the application source code
COPY . .

# 7. Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /sendtoanki

EXPOSE 8080

CMD ["/sendtoanki"]