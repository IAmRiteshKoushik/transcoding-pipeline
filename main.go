package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/azure-storage-queue-go/azqueue"
	"github.com/joho/godotenv"
)

// parseConnectionString extracts account name and key from the connection string
func parseConnectionString(connStr string) (accountName, accountKey string, err error) {
	if connStr == "" {
		return "", "", fmt.Errorf("connection string is empty")
	}

	parts := strings.Split(connStr, ";")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("connection string has too few parts: %v", parts)
	}

	for _, part := range parts {
		if strings.HasPrefix(part, "AccountName=") {
			accountName = strings.TrimPrefix(part, "AccountName=")
		}
		if strings.HasPrefix(part, "AccountKey=") {
			accountKey = strings.TrimPrefix(part, "AccountKey=")
		}
	}

	if accountName == "" || accountKey == "" {
		return "", "", fmt.Errorf("missing AccountName or AccountKey in connection string")
	}

	return accountName, accountKey, nil
}

func main() {
	envErr := godotenv.Load()
	log.SetOutput(os.Stdout)
	if envErr != nil {
		log.Println("Error loading .env file")
	}
	log.Println("Env read successfully")

	connectionString := os.Getenv("AZURE_STORAGE_CONNECTION_STRING")
	queueName := os.Getenv("QUEUE_NAME")
	resolution := os.Getenv("RESOLUTION")
	outputContainer := "output"

	// Parse credentials safely
	accountName, accountKey, err := parseConnectionString(connectionString)
	if err != nil {
		log.Fatalf("Failed to parse connection string: %v", err)
	}
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		log.Fatal("Invalid credentials:", err)
	}
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	log.Println("Pipeline established")

	// Setup queue credentials
	queueUrlString := fmt.Sprintf("https://%s.queue.core.windows.net/%s", accountName, queueName)
	queueURL, err := url.Parse(queueUrlString)
	if err != nil {
		log.Fatal("Invalid queueUrl")
	}
	queue := azqueue.NewQueueURL(*queueURL, pipeline)
	log.Println("Queue established")

	for {
		log.Println("Listening for messages")
		messagesURL := queue.NewMessagesURL()
		dequeueResp, err := messagesURL.Dequeue(
			context.Background(),
			1,              // maxMessages
			30*time.Second, // visibility timeout
		)
		if err != nil {
			log.Println("Dequeue error:", err)
			continue
		}
		if dequeueResp.NumMessages() == 0 {
			continue
		}

		msg := dequeueResp.Message(0)
		var data map[string]string
		if err := json.Unmarshal([]byte(msg.Text), &data); err != nil {
			log.Println("Unmarshal error:", err)
			continue
		}

		videoName := data["video_name"]
		inputFile := videoName
		outputFile := strings.TrimSuffix(videoName, ".mp4") + "_" + resolution + ".mp4"
		outputBlobName := outputFile

		// Download video from input container
		blobURLString := fmt.Sprintf("https://%s.blob.core.windows.net/input/%s", accountName, videoName)
		blobURL, err := url.Parse(blobURLString)
		if err != nil {
			log.Println("Could not parse blob-url: ", err)
			continue
		}
		blob := azblob.NewBlobURL(*blobURL, pipeline)
		downloadResp, err := blob.Download(context.Background(), 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
		if err != nil {
			log.Println("Download error:", err)
			continue
		}

		file, err := os.Create(inputFile)
		if err != nil {
			log.Println("File creation error:", err)
			continue
		}
		_, err = file.ReadFrom(downloadResp.Body(azblob.RetryReaderOptions{}))
		file.Close()
		if err != nil {
			log.Println("Download write error:", err)
			continue
		}

		// Transcode
		scale := fmt.Sprintf("scale=-2:%s", resolution)
		bitrate := "1M"
		if resolution == "720" {
			bitrate = "2M"
		}
		cmd := exec.Command("ffmpeg", "-i", inputFile, "-vf", scale, "-c:v", "libx264", "-b:v", bitrate, "-c:a", "aac", "-b:a", "128k", "-y", outputFile)
		if err := cmd.Run(); err != nil {
			log.Println("FFmpeg error:", err)
			continue
		}

		// Upload result
		outputBlobURLString := fmt.Sprintf(
			"https://%s.blob.core.windows.net/%s/%s",
			accountName,
			outputContainer,
			outputBlobName,
		)
		outputBlobURL, err := url.Parse(outputBlobURLString)
		if err != nil {
			log.Println("Upload error: ", err)
			continue
		}
		outputBlob := azblob.NewBlobURL(*outputBlobURL, pipeline)
		file, err = os.Open(outputFile)
		if err != nil {
			log.Println("Output file open error:", err)
			continue
		}
		_, err = azblob.UploadFileToBlockBlob(
			context.Background(),
			file,
			outputBlob.ToBlockBlobURL(),
			azblob.UploadToBlockBlobOptions{
				BlockSize:   4 * 1024 * 1024, // 4MB block
				Parallelism: 16,
			},
		)
		file.Close()
		if err != nil {
			log.Println("Upload error:", err)
			continue
		}

		// Cleanup
		os.Remove(inputFile)
		os.Remove(outputFile)
		messageIDURL := messagesURL.NewMessageIDURL(msg.ID)
		if _, err := messageIDURL.Delete(context.Background(), msg.PopReceipt); err != nil {
			log.Println("Delete message error:", err)
			continue
		}

		fmt.Printf("Transcoded %s to %sp\n", videoName, resolution)
	}
}
