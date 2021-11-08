package main

import (
	"log"
	"net/http"
    "flag"
    "fmt"
)

func main() {
    portPtr := flag.Int("port", 8080, "listening port")
    jobsPtr := flag.Int("jobs", 5, "number of buffered send jobs")
    workersPtr := flag.Int("workers", 3, "number of send worker threads")

    flag.Parse()

    portNum := *portPtr

    log.Printf("###### Starting MQ-Go server ######")
    log.Printf("Port: %d\n", portNum)
    log.Printf("Jobs: %d\n", *jobsPtr)
    log.Printf("Workers: %d\n", *workersPtr)

	serverObj := NewServer(*jobsPtr) // 5 jobs
	http.HandleFunc("/connect/", serverObj.ConnectHandler)
	http.HandleFunc("/send/", serverObj.SendHandler)
	http.HandleFunc("/dashboard/", serverObj.DashboardHandler)
    http.HandleFunc("/disconnect/", serverObj.DisconnectHandler)

	go serverObj.SendManager(*workersPtr) // 3 concurrent workers

    portString := fmt.Sprintf(":%d", portNum)
	log.Fatal(http.ListenAndServe(portString, nil))
}
