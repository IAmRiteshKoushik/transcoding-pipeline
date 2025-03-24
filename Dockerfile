# Use an official Go runtime as a base image
FROM golang:1.24 AS builder

# Set working directory
WORKDIR /app

# Copy source code
COPY main.go .
COPY go.mod .
COPY go.sum .

# Build the application
RUN GOOS=linux GOARCH=amd64 go build -o video-transcoder main.go

# Use a minimal base image with FFmpeg
FROM ubuntu:22.04

# Install FFmpeg
RUN apt-get update && apt-get install -y ffmpeg && rm -rf /var/lib/apt/lists/*

# Copy the binary from the builder stage
COPY --from=builder /app/video-transcoder /usr/local/bin/

# Set environment variables (override in runtime)
ENV AZURE_STORAGE_CONNECTION_STRING=""
ENV QUEUE_NAME="transcode-480p-videos"
ENV RESOLUTION="480"

# Run the binary
CMD ["video-transcoder"]
