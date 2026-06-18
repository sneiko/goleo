package server

import (
	"context"
	"errors"
	"sync"
)

var errQueueFull = errors.New("request queue is full")

type queuedRequest struct {
	ready chan struct{}
}

type interfaceQueue struct {
	maxConcurrency int
	maxQueue       int
	running        int
	waiting        []*queuedRequest
	mu             sync.Mutex
}

type queueManager struct {
	defaultMaxConcurrency int
	defaultMaxQueue       int
	queues                map[string]*interfaceQueue
	mu                    sync.Mutex
}

func newQueueManager(maxConcurrency, maxQueue int) *queueManager {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	if maxQueue < 0 {
		maxQueue = 0
	}

	return &queueManager{
		defaultMaxConcurrency: maxConcurrency,
		defaultMaxQueue:       maxQueue,
		queues:                map[string]*interfaceQueue{},
	}
}

func (manager *queueManager) getQueue(interfaceID string) *interfaceQueue {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	queue, ok := manager.queues[interfaceID]
	if ok {
		return queue
	}

	queue = &interfaceQueue{
		maxConcurrency: manager.defaultMaxConcurrency,
		maxQueue:       manager.defaultMaxQueue,
	}
	manager.queues[interfaceID] = queue
	return queue
}

func (manager *queueManager) acquire(ctx context.Context, interfaceID string) (release func(), queued bool, err error) {
	queue := manager.getQueue(interfaceID)
	return queue.acquire(ctx)
}

func (q *interfaceQueue) acquire(ctx context.Context) (release func(), queued bool, err error) {
	q.mu.Lock()
	if q.running < q.maxConcurrency {
		q.running++
		q.mu.Unlock()
		return q.release, false, nil
	}

	if q.maxQueue >= 0 && len(q.waiting) >= q.maxQueue {
		q.mu.Unlock()
		return nil, false, errQueueFull
	}

	request := &queuedRequest{ready: make(chan struct{})}
	q.waiting = append(q.waiting, request)
	q.mu.Unlock()

	select {
	case <-ctx.Done():
		q.dequeue(request)
		return nil, true, ctx.Err()
	case <-request.ready:
		return q.release, true, nil
	}
}

func (q *interfaceQueue) release() {
	q.mu.Lock()
	if q.running > 0 {
		q.running--
	}

	q.startNext()
	q.mu.Unlock()
}

func (q *interfaceQueue) startNext() {
	if q.running >= q.maxConcurrency || len(q.waiting) == 0 {
		return
	}

	request := q.waiting[0]
	q.waiting = q.waiting[1:]
	q.running++
	close(request.ready)
}

func (q *interfaceQueue) dequeue(target *queuedRequest) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for index, item := range q.waiting {
		if item == target {
			q.waiting = append(q.waiting[:index], q.waiting[index+1:]...)
			return
		}
	}
}
