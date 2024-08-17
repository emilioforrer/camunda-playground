package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/camunda/zeebe/clients/go/v8/pkg/entities"
	"github.com/camunda/zeebe/clients/go/v8/pkg/worker"
	"github.com/camunda/zeebe/clients/go/v8/pkg/zbc"
)

const defaultZeebeAddr = "0.0.0.0:26500"

func main() {
	log.Println("Starting Go worker")

	// Retrieve configuration
	gatewayAddr := os.Getenv("ZEEBE_ADDRESS")
	plainText := false

	if gatewayAddr == "" {
		gatewayAddr = defaultZeebeAddr
		plainText = true
	}

	// Create Zeebe client
	zbClient, err := zbc.NewClient(&zbc.ClientConfig{
		GatewayAddress:         gatewayAddr,
		UsePlaintextConnection: plainText,
	})
	if err != nil {
		log.Fatalf("Failed to create Zeebe client: %v", err)
	}

	// Context for managing worker lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	// WaitGroup to ensure graceful shutdown
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the worker in a separate Goroutine
	go startWorker(ctx, zbClient, &wg)

	// Graceful shutdown on SIGINT or SIGTERM
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown
	log.Println("Shutdown signal received, closing worker...")
	cancel()

	// Wait for the worker to close gracefully
	wg.Wait()
	log.Println("Worker closed, exiting.")
}

func startWorker(ctx context.Context, zbClient zbc.Client, wg *sync.WaitGroup) {
	defer wg.Done() // Signal that the worker has completed
	log.Println("Starting Payment Service Worker")
	jobWorker := zbClient.NewJobWorker().JobType("go-service").Handler(handleJob).Open()
	defer func() {
		jobWorker.Close()
		jobWorker.AwaitClose()
		log.Println("Job worker closed")
	}()

	<-ctx.Done()
}

func handleJob(client worker.JobClient, job entities.Job) {
	jobKey := job.GetKey()

	headers, err := job.GetCustomHeadersAsMap()
	if err != nil {
		log.Printf("Failed to get custom headers for job %d: %v", jobKey, err)
		failJob(client, job)
		return
	}

	variables, err := job.GetVariablesAsMap()
	if err != nil {
		log.Printf("Failed to get variables for job %d: %v", jobKey, err)
		failJob(client, job)
		return
	}

	variables["paymentStatus"] = "COMPLETED"
	request, err := client.NewCompleteJobCommand().JobKey(jobKey).VariablesFromMap(variables)
	if err != nil {
		log.Printf("Failed to complete job %d: %v", jobKey, err)
		failJob(client, job)
		return
	}

	log.Printf("Processing job %d of type %s", jobKey, job.Type)
	log.Printf("Order ID: %s, Payment status: %s, Payment method: %s", variables["orderId"], variables["paymentStatus"], headers["paymentMethod"])

	ctx := context.Background()
	_, err = request.Send(ctx)
	if err != nil {
		log.Printf("Failed to send job completion for job %d: %v", jobKey, err)
		failJob(client, job)
		return
	}

	log.Printf("Successfully completed job %d", jobKey)
}

func failJob(client worker.JobClient, job entities.Job) {
	jobKey := job.GetKey()
	log.Printf("Failing job %d", jobKey)

	ctx := context.Background()
	_, err := client.NewFailJobCommand().JobKey(jobKey).Retries(job.Retries - 1).Send(ctx)
	if err != nil {
		log.Printf("Failed to fail job %d: %v", jobKey, err)
	}
}
