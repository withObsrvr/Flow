// test_gcs.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Get bucket name from command line
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_gcs.go <bucket-name>")
		os.Exit(1)
	}
	bucketName := os.Args[1]

	// Print environment variables
	fmt.Println("GOOGLE_APPLICATION_CREDENTIALS:", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	// Parse bucket name and prefix
	parts := strings.SplitN(bucketName, "/", 2)
	bucket := parts[0]
	prefix := ""
	if len(parts) > 1 {
		prefix = parts[1]
	}

	fmt.Printf("Bucket: %s\n", bucket)
	fmt.Printf("Prefix: %s\n", prefix)

	// Instead of using the GCS client directly, use gsutil command
	fmt.Printf("\nRunning: gsutil ls gs://%s/%s\n", bucket, prefix)
	cmd := exec.Command("gsutil", "ls", fmt.Sprintf("gs://%s/%s", bucket, prefix))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Error running gsutil: %v", err)
	}
}
