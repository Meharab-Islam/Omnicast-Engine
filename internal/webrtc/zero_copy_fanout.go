package webrtc

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// SharedPacket wraps an RTP packet with an immutable payload and atomic reference count,
// enabling zero-copy distribution to thousands of subscribers without per-viewer memory allocations.
type SharedPacket struct {
	Header    rtp.Header
	Payload   []byte // Shared read-only slice across all viewers
	Raw       []byte // Serialized raw RTP packet buffer
	refCount  int32
	onRelease func(*SharedPacket)
}

// Retain increments the reference count of the SharedPacket.
func (sp *SharedPacket) Retain() {
	atomic.AddInt32(&sp.refCount, 1)
}

// Release decrements the reference count and recycles the packet buffer when count reaches zero.
func (sp *SharedPacket) Release() {
	if atomic.AddInt32(&sp.refCount, -1) == 0 && sp.onRelease != nil {
		sp.onRelease(sp)
	}
}

// RefCount returns the current reference count.
func (sp *SharedPacket) RefCount() int32 {
	return atomic.LoadInt32(&sp.refCount)
}

// Subscriber defines an egress target for RTP packets (e.g., Viewer WebRTC TrackLocal).
type Subscriber struct {
	ID        string
	Track     *webrtc.TrackLocalStaticRTP
	Queue     chan *SharedPacket
	closed    chan struct{}
	closeOnce sync.Once
}

// NewSubscriber creates a new Subscriber with a bounded non-blocking queue.
func NewSubscriber(id string, track *webrtc.TrackLocalStaticRTP, queueSize int) *Subscriber {
	if queueSize <= 0 {
		queueSize = 256
	}
	return &Subscriber{
		ID:     id,
		Track:  track,
		Queue:  make(chan *SharedPacket, queueSize),
		closed: make(chan struct{}),
	}
}

// Close gracefully closes the subscriber channel.
func (s *Subscriber) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
	})
}

// FanOutDispatcher distributes RTP packets to thousands of subscribers with zero-copy forwarding
// using a bounded worker pool instead of per-viewer goroutines.
type FanOutDispatcher struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber
	workQueue   chan dispatchTask
	numWorkers  int
	stopCh      chan struct{}
	stopped     bool
	pool        sync.Pool
}

type dispatchTask struct {
	packet     *SharedPacket
	subscriber *Subscriber
}

// NewFanOutDispatcher creates a new high-throughput zero-copy dispatcher with a fixed worker pool.
func NewFanOutDispatcher(numWorkers int, queueSize int) *FanOutDispatcher {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU() * 2
		if numWorkers < 4 {
			numWorkers = 4
		}
	}
	if queueSize <= 0 {
		queueSize = 4096
	}

	d := &FanOutDispatcher{
		subscribers: make(map[string]*Subscriber),
		workQueue:   make(chan dispatchTask, queueSize),
		numWorkers:  numWorkers,
		stopCh:      make(chan struct{}),
		pool: sync.Pool{
			New: func() any {
				return &SharedPacket{
					Raw: make([]byte, 1500),
				}
			},
		},
	}

	d.startWorkers()
	return d
}

func (d *FanOutDispatcher) startWorkers() {
	for i := 0; i < d.numWorkers; i++ {
		go func() {
			for {
				select {
				case <-d.stopCh:
					return
				case task, ok := <-d.workQueue:
					if !ok {
						return
					}
					d.processTask(task)
				}
			}
		}()
	}
}

func (d *FanOutDispatcher) processTask(task dispatchTask) {
	defer task.packet.Release()

	if task.subscriber == nil || task.subscriber.Track == nil {
		return
	}

	// Non-blocking write to subscriber queue: drop for slow subscriber to protect broadcast performance
	select {
	case <-task.subscriber.closed:
		return
	case task.subscriber.Queue <- task.packet:
		task.packet.Retain() // Retain reference while in subscriber queue
	default:
		// Queue full: packet dropped for lagging client (drop-tail policy)
	}
}

// Subscribe registers a new subscriber for zero-copy RTP delivery.
func (d *FanOutDispatcher) Subscribe(sub *Subscriber) error {
	if sub == nil || sub.ID == "" {
		return errors.New("invalid subscriber")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return errors.New("dispatcher is stopped")
	}

	d.subscribers[sub.ID] = sub
	return nil
}

// Unsubscribe removes a subscriber and closes their queue.
func (d *FanOutDispatcher) Unsubscribe(subscriberID string) {
	d.mu.Lock()
	sub, exists := d.subscribers[subscriberID]
	if exists {
		delete(d.subscribers, subscriberID)
	}
	d.mu.Unlock()

	if exists && sub != nil {
		sub.Close()
	}
}

// GetSubscriberCount returns the number of active subscribers.
func (d *FanOutDispatcher) GetSubscriberCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.subscribers)
}

// Broadcast dispatches a single RTP packet to all subscribers with zero-copy memory sharing.
// filter is an optional predicate to selectively skip subscribers (e.g., Dynamic Viewport / Server-Side Pausing).
func (d *FanOutDispatcher) Broadcast(packet *rtp.Packet, rawBytes []byte, filter func(subscriberID string) bool) int {
	if packet == nil && len(rawBytes) == 0 {
		return 0
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.stopped || len(d.subscribers) == 0 {
		return 0
	}

	// Create a shared packet reference
	sp := d.pool.Get().(*SharedPacket)
	if packet != nil {
		sp.Header = packet.Header
		sp.Payload = packet.Payload
	}
	if len(rawBytes) > 0 {
		if cap(sp.Raw) < len(rawBytes) {
			sp.Raw = make([]byte, len(rawBytes))
		} else {
			sp.Raw = sp.Raw[:len(rawBytes)]
		}
		copy(sp.Raw, rawBytes)
	}
	sp.refCount = 1
	sp.onRelease = func(p *SharedPacket) {
		d.pool.Put(p)
	}

	sentCount := 0
	for id, sub := range d.subscribers {
		// Apply filter for dynamic viewport / selective track pausing
		if filter != nil && !filter(id) {
			continue // Paused / not visible for this viewer
		}

		sp.Retain()
		task := dispatchTask{
			packet:     sp,
			subscriber: sub,
		}

		select {
		case d.workQueue <- task:
			sentCount++
		default:
			// Dispatch queue full — release reference
			sp.Release()
		}
	}

	// Release the initial reference
	sp.Release()
	return sentCount
}

// Stop terminates the worker pool and shuts down the dispatcher.
func (d *FanOutDispatcher) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.stopped {
		d.stopped = true
		close(d.stopCh)
		for _, sub := range d.subscribers {
			sub.Close()
		}
		d.subscribers = make(map[string]*Subscriber)
	}
}
