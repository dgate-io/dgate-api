package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dgate-io/dgate-api/pkg/scheduler"
	"github.com/dgate-io/dgate-api/pkg/util/heap"
	"go.uber.org/zap"
)

type TCache interface {
	Bucket(string) Bucket
	BucketWithOpts(string, BucketOptions) Bucket
	Clear()
}

type Bucket interface {
	Get(string) (any, bool)
	Set(string, any)
	Len() int
	SetWithTTL(string, any, time.Duration)
	Delete(string) bool
}

type BucketOptions struct {
	// TTL is the time after which the key will be deleted from the cache.
	DefaultTTL time.Duration
	// MaxItems is the maximum number of items that can be stored in the cache.
	// If set to 0, there is no limit.
	MaxItems int
}

type cacheImpl struct {
	mutex    *sync.RWMutex
	buckets  map[string]Bucket
	sch      scheduler.Scheduler
	interval time.Duration
}

type bucketImpl struct {
	name  string
	opts  *BucketOptions
	mutex *sync.RWMutex

	items      map[string]*cacheEntry
	ttlQueue   *heap.Heap[int64, *cacheEntry]
	limitQueue *heap.Heap[int64, *cacheEntry]
	// limitQueue queue.Queue[*cacheEntry]
}

type cacheEntry struct {
	key   string
	value any
	exp   time.Time
}

type CacheOptions struct {
	CheckInterval time.Duration
	Logger        *zap.Logger
}

var (
	ErrNotFound = errors.New("key not found")
	ErrMaxItems = errors.New("max items reached")
)

func NewWithOpts(opts CacheOptions) TCache {
	sch := scheduler.New(scheduler.Options{
		Logger:   opts.Logger,
		Interval: opts.CheckInterval,
		AutoRun:  true,
	})

	if opts.CheckInterval == 0 {
		opts.CheckInterval = time.Second * 5
	}

	return &cacheImpl{
		sch:      sch,
		mutex:    &sync.RWMutex{},
		buckets:  make(map[string]Bucket),
		interval: opts.CheckInterval,
	}
}

func New() TCache {
	return NewWithOpts(CacheOptions{})
}

func (cache *cacheImpl) newBucket(
	name string,
	opts BucketOptions,
) Bucket {
	cache.sch.ScheduleTask(name, scheduler.TaskOptions{
		Interval: cache.interval,
		TaskFunc: func(_ context.Context) {
			cache.mutex.RLock()
			b := cache.buckets[name].(*bucketImpl)
			cache.mutex.RUnlock()

			b.mutex.Lock()
			defer b.mutex.Unlock()
			for {
				if t, v, ok := b.ttlQueue.Peak(); !ok {
					break
				} else {
					// if the expiration time is not the same as the value's expiration time,
					// it means the value has been updated, so we pop it from the queue
					if t != v.exp.UnixMilli() {
						b.ttlQueue.Pop()
						continue
					}
					// if the expiration time is in the future, we break
					if v.exp.After(time.Now()) {
						break
					}
					// if the expiration time is in the past, we pop the value from the queue
					b.ttlQueue.Pop()
					delete(b.items, v.key)
				}
			}
		},
	})

	return &bucketImpl{
		name:       name,
		opts:       &opts,
		mutex:      &sync.RWMutex{},
		items:      make(map[string]*cacheEntry),
		ttlQueue:   heap.NewHeap[int64, *cacheEntry](heap.MinHeapType),
		limitQueue: heap.NewHeap[int64, *cacheEntry](heap.MinHeapType),
	}
}

func (c *cacheImpl) BucketWithOpts(name string, opts BucketOptions) Bucket {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if b, ok := c.buckets[name]; ok {
		return b
	}
	b := c.newBucket(name, opts)
	c.buckets[name] = b
	return b
}

func (c *cacheImpl) Bucket(name string) Bucket {
	return c.BucketWithOpts(name, BucketOptions{})
}

func (b *bucketImpl) Get(key string) (any, bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	if v, ok := b.items[key]; ok {
		if !v.exp.IsZero() && !v.exp.After(time.Now()) {
			return nil, false
		}
		return v.value, true
	}
	return nil, false
}

func (b *bucketImpl) Set(key string, value any) {
	b.SetWithTTL(key, value, b.opts.DefaultTTL)
}

func (b *bucketImpl) Len() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return len(b.items)
}

func (b *bucketImpl) SetWithTTL(key string, value any, ttl time.Duration) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if v, ok := b.items[key]; !ok {
		if b.opts.MaxItems > 0 && len(b.items) >= b.opts.MaxItems {
			// remove items with TTLs first
			_, ce, ok := b.ttlQueue.Pop()
			if !ok {
				// if no items with TTLs, remove items from with no TTLs
				if _, ce, ok = b.limitQueue.Pop(); !ok {
					panic("inconsistent state: no items in limit or ttl queue")
				}
			}
			delete(b.items, ce.key)
		}
	} else {
		v.value = value
		if ttl > 0 {
			v.exp = time.Now().Add(ttl)
			b.ttlQueue.Push(v.exp.UnixMilli(), v)
		} else {
			v.exp = time.Time{}
			b.limitQueue.Push(time.Now().UnixMilli(), v)
		}
		return
	}
	e := &cacheEntry{
		key:   key,
		value: value,
	}
	e.exp = time.Time{}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}

	if ttl > 0 {
		b.ttlQueue.Push(e.exp.UnixMilli(), e)
	} else if b.opts.MaxItems > 0 {
		b.limitQueue.Push(time.Now().UnixMilli(), e)
	}
	b.items[key] = e
}

func (b *bucketImpl) Delete(key string) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.delete(key)
}

func (b *bucketImpl) delete(key string) bool {
	if _, ok := b.items[key]; !ok {
		return false
	}
	delete(b.items, key)
	return true
}

func (cache *cacheImpl) Clear() {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	for _, b := range cache.buckets {
		if bkt, ok := b.(*bucketImpl); ok {
			bkt.mutex.Lock()
			bkt.items = make(map[string]*cacheEntry)
			bkt.ttlQueue = heap.NewHeap[int64, *cacheEntry](heap.MinHeapType)
			bkt.limitQueue = heap.NewHeap[int64, *cacheEntry](heap.MinHeapType)
			bkt.mutex.Unlock()
		}
	}
}
