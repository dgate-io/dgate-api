package keylock

import (
	"sync"
)

type KeyLock struct {
	locks   map[string]*sync.RWMutex
	mapLock sync.RWMutex // to make the map safe concurrently
}

type UnlockFunc func()

func NewKeyLock() *KeyLock {
	return &KeyLock{locks: make(map[string]*sync.RWMutex, 32)}
}

func (l *KeyLock) getLockBy(key string) *sync.RWMutex {
	if mtx, ok := l.findLock(key); ok {
		return mtx
	}

	l.mapLock.Lock()
	defer l.mapLock.Unlock()
	ret := &sync.RWMutex{}
	l.locks[key] = ret
	return ret
}

func (l *KeyLock) findLock(key string) (*sync.RWMutex, bool) {
	l.mapLock.RLock()
	defer l.mapLock.RUnlock()

	if ret, found := l.locks[key]; found {
		return ret, true
	}
	return nil, false
}

func (l *KeyLock) RLock(key string) UnlockFunc {
	mtx := l.getLockBy(key)
	mtx.RLock()
	return mtx.RUnlock
}

func (l *KeyLock) Lock(key string) UnlockFunc {
	mtx := l.getLockBy(key)
	mtx.Lock()
	return mtx.Unlock
}

func (l *KeyLock) RLockMain() UnlockFunc {
	l.mapLock.RLock()
	return l.mapLock.RUnlock
}

func (l *KeyLock) LockMain() UnlockFunc {
	l.mapLock.Lock()
	return l.mapLock.Unlock
}
