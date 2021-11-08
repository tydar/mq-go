package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

// Incoming message structs for JSON unmarshalling
// These structs should never be manually created, so no helper functions
// Instead they should always be filled by UmarshalJSON calls

type ConnectMessage struct {
	ClientURL string
	Mode      string
}

type SendMessage struct {
	ID   int
	Body string
}

type DisconnectMessage struct {
	ID int
}

// core message queue type and functions

type Queue struct {
	sync.Mutex
	queue []string
}

func NewQueue() *Queue {
	return &Queue{queue: make([]string, 0)}
}

func (q *Queue) Add(item string) {
	q.Lock()
	defer q.Unlock()
	q.queue = append(q.queue, item)
}

func (q *Queue) Pop() (string, error) {
	q.Lock()
	defer q.Unlock()
	if len(q.queue) == 0 {
		return "", errors.New("Cannot pop empty queue.")
	}
	item := q.queue[0]
	q.queue = q.queue[1:]
	return item, nil
}

func (q *Queue) Len() int {
	q.Lock()
	defer q.Unlock()
	return len(q.queue)
}

// Connections table type and functions
type Connections struct {
	sync.Mutex
	last    int            // last ID assigned
	writers map[int]string // map of ID -> interface URL
	readers map[int]string // ""
}

func NewConnections() *Connections {
	return &Connections{
		last:    0,
		writers: make(map[int]string),
		readers: make(map[int]string),
	}
}

func (c *Connections) AddConnection(url, mode string) (int, error) {
	// AddConnection first waits for access to the structure
	// then it checks to be sure a valid mode has been requested
	// finally it updates the stored value for last and adds the connection
	// to the appropriate map
	c.Lock()
	defer c.Unlock()
	current := c.last + 1
	if mode == "send" {
		c.writers[current] = url
		c.last = current
		return c.last, nil
	} else if mode == "receive" {
		c.readers[current] = url
		c.last = current
		return c.last, nil
	} else {
		return 0, errors.New(fmt.Sprintf("Request Error: Incorrect mode string %s.", mode))
	}
}

func (c *Connections) Disconnect(id int) error {
	c.Lock()
	defer c.Unlock()
	_, wprs := c.writers[id]
	if wprs {
		delete(c.writers, id)
		return nil
	}

	_, rprs := c.readers[id]
	if rprs {
		delete(c.readers, id)
		return nil
	}
	return errors.New("Could not disconnect client. No such connection in map.")
}

// not sure that these getter wrappers are "necessary" with a mutex or not
// but it does seem like it works correctly this way
func (c *Connections) Writers() map[int]string {
	c.Lock()
	defer c.Unlock()
	return c.writers
}

func (c *Connections) Readers() map[int]string {
	c.Lock()
	defer c.Unlock()
	return c.readers
}

// Main Server type
// wraps a Queue and a Connections instance, which are the total state of the system
// also wraps a JobCount & Jobs channel for distributing work to worker threads

type Server struct {
	Queue       *Queue
	Connections *Connections
	JobCount    int
	Jobs        chan MsgJob
}

func NewServer(numJobs int) *Server {
	return &Server{
		Queue:       NewQueue(),
		Connections: NewConnections(),
		JobCount:    0,
		Jobs:        make(chan MsgJob, numJobs),
	}
}

// HTTP request handlers
// I probably want to refactor these to support both a GET and a POST.

func (s *Server) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	// first receive the JSON body as a string
	buf := new(strings.Builder)
	_, err := io.Copy(buf, r.Body)

	if err != nil {
		log.Printf("Error copying request body: %s.\n", err.Error())
		return
	}

	bodyString := buf.String()

	// then Unmarshal into a ConnectMessage struct
	var reqObj ConnectMessage
	json.Unmarshal([]byte(bodyString), &reqObj)

	// then add a new connection
	id, err := s.Connections.AddConnection(reqObj.ClientURL, reqObj.Mode)
	if err != nil {
		log.Printf("Error adding connection: %s.\n", err.Error())
		return
	}

	// and respond with the ID
	fmt.Fprintf(w, "{'id': %d}\n", id)
}

func (s *Server) SendHandler(w http.ResponseWriter, r *http.Request) {
	// first receive the JSON body as a string
	buf := new(strings.Builder)
	_, err := io.Copy(buf, r.Body)

	if err != nil {
		log.Printf("Error copying request body: %s.\n", err.Error())
		return
	}

	bodyString := buf.String()

	// Unmarshal to a SendMessage struct
	var reqObj SendMessage
	json.Unmarshal([]byte(bodyString), &reqObj)

	// maybe add a check for the ID to match a sender here

	// enqueue
	s.Queue.Add(reqObj.Body)
	fmt.Fprintf(w, "{\"status\": \"Message enqueued\", \"id\": %d}\n", reqObj.ID)
}

func (s *Server) DisconnectHandler(w http.ResponseWriter, r *http.Request) {
	// first receive the JSON body as a string
	buf := new(strings.Builder)
	_, err := io.Copy(buf, r.Body)

	if err != nil {
		log.Printf("Error copying request body: %s.\n", err.Error())
		return
	}

	bodyString := buf.String()

	// Unmarshal to a DisconnectMessage struct
	var reqObj DisconnectMessage
	json.Unmarshal([]byte(bodyString), &reqObj)

	err2 := s.Connections.Disconnect(reqObj.ID)
	if err2 != nil {
		log.Printf("Error disconnecting ID %d: %s.\n", reqObj.ID, err2.Error())
		fmt.Fprintf(w, "{\"status\":\"Error: no such connection.\", \"id\": %d}\n", reqObj.ID)
		return
	}
	fmt.Fprintf(w, "{\"status\":\"Disconnected.\", \"id\": %d}\n", reqObj.ID)
}

func (s *Server) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	readers := s.Connections.Readers()
	writers := s.Connections.Writers()
	fmt.Fprintf(w,
		"<html><body><h1>Dashboard</h1><p>Readers: %d</p><p>Writers: %d</p><p>Queue len: %d</p><p>Completed sends: %d</p></body></html>",
		len(readers),
		len(writers),
		s.Queue.Len(),
		s.JobCount,
	)
}

// workers
// worker pool implemented using buffered channels
// pattern adopted from https://gobyexample.com/worker-pools

type MsgJob struct {
	JobID int
	Msg   string
	Dests map[int]string
}

func SendWorker(id int, jobs <-chan MsgJob) {
	for j := range jobs {
		for _, v := range j.Dests {
			resp, err := http.Post(v, "text/plain", strings.NewReader(j.Msg))
			log.Printf("Worker %d Job %d: Request sent to %s.", id, j.JobID, v)
			if err != nil {
				log.Fatal(err.Error())
			}
			defer resp.Body.Close()
			barr, err2 := io.ReadAll(resp.Body)
			if err2 != nil {
				log.Fatal(err2.Error())
			}
			log.Printf("Worker %d Job %d: Response received from %s with body %s.\n", id, j.JobID, v, string(barr))
		}
	}
}

func (s *Server) SendManager(numWorkers int) {
	// IFF items in queue AND receivers in receiver map
	// THEN Pop a message off the queue and send it to every receiver
	rs := s.Connections.Readers()
	for w := 0; w < numWorkers; w++ {
		go SendWorker(w, s.Jobs)
	}
	for {
		rs = s.Connections.Readers()
		if s.Queue.Len() > 0 && len(rs) > 0 {
			msg, err := s.Queue.Pop()
			if err != nil {
				log.Fatal(err.Error())
			} else {
				j := MsgJob{
					JobID: s.JobCount,
					Msg:   msg,
					Dests: rs,
				}
				s.JobCount++
				s.Jobs <- j
			}
		}
	}
}
