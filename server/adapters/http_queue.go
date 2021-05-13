package adapters

import (
	"errors"
	"fmt"
	"github.com/joncrlsn/dque"
	"net/http"
	"time"
)

const requestsPerPersistedFile = 2000

//ErrQueueClosed is a error in case when queue has been already closed
var ErrQueueClosed = errors.New("queue is closed")

//QueuedRequest is a dto for enqueueing
type QueuedRequest struct {
	HTTPReq      *http.Request
	Retry        int
	DequeuedTime time.Time
}

//QueuedRequestBuilder creates and returns a new *adapters.QueuedRequest (must be pointer).
// This is used when we load a segment of the queue from disk.
func QueuedRequestBuilder() interface{} {
	return &QueuedRequest{}
}

//PersistentQueue is a queue (persisted on file system) with requests
type PersistentQueue struct {
	queue *dque.DQue
}

//NewPersistentQueue returns configured PersistentQueue instance
func NewPersistentQueue(queueName, fallbackDir string) (*PersistentQueue, error) {
	queue, err := dque.NewOrOpen(queueName, fallbackDir, requestsPerPersistedFile, QueuedRequestBuilder)
	if err != nil {
		return nil, fmt.Errorf("Error opening/creating HTTP requests queue [%s] in dir [%s]: %v", queueName, fallbackDir, err)
	}

	return &PersistentQueue{queue: queue}, nil
}

//Add puts HTTP request to the queue
func (pq *PersistentQueue) Add(httpReq *http.Request) error {
	return pq.Retry(&QueuedRequest{HTTPReq: httpReq, DequeuedTime: time.Now().UTC(), Retry: 0})
}

//Retry puts request to the queue with retryCount
func (pq *PersistentQueue) Retry(req *QueuedRequest) error {
	return pq.queue.Enqueue(req)
}

//DequeueBlock waits when enqueued request is ready and return it
func (pq *PersistentQueue) DequeueBlock() (*QueuedRequest, error) {
	iface, err := pq.queue.DequeueBlock()
	if err != nil {
		if err == dque.ErrQueueClosed {
			err = ErrQueueClosed
		}
		return nil, err
	}

	wrappedReq, ok := iface.(*QueuedRequest)
	if !ok {
		return nil, fmt.Errorf("Dequeued object is not a QueuedRequest instance. Type is: %T", iface)
	}

	return wrappedReq, nil
}

//Close closes underlying persistent queue
func (pq *PersistentQueue) Close() error {
	return pq.queue.Close()
}