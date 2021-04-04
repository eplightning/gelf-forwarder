package util

import (
	"net"
	"sync"
)

type ConnectionMap struct {
	mutex  sync.Mutex
	cm     map[int]net.Conn
	nextId int
}

func NewConnectionMap() *ConnectionMap {
	return &ConnectionMap{
		cm: make(map[int]net.Conn),
	}
}

func (m *ConnectionMap) Add(conn net.Conn) int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	id := m.nextId
	m.nextId++

	m.cm[id] = conn
	return id
}

func (m *ConnectionMap) CloseAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, conn := range m.cm {
		conn.Close()
	}

	m.cm = make(map[int]net.Conn)
	m.nextId = 0
}

func (m *ConnectionMap) Close(id int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	conn, found := m.cm[id]
	if found {
		conn.Close()
		delete(m.cm, id)
	}
}
