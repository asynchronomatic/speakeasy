package proxy

import (
	"sync"
	"time"
)

type Metric struct {
	RequestTime  float64
	RequestCount uint64
}

type Metrics struct {
	lock    sync.RWMutex
	ingress map[string]*Metric
	egress  map[string]*Metric
}

func (m *Metrics) ObserveIngress(id string, d time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	metric, ok := m.ingress[id]
	if !ok {
		metric = &Metric{}
		m.ingress[id] = metric
	}
	metric.RequestTime += d.Seconds()
	metric.RequestCount++
}

func (m *Metrics) GetIngress(id string) Metric {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if metric, ok := m.ingress[id]; ok {
		return *metric
	}
	return Metric{}
}

func (m *Metrics) ObserveEgress(id string, d time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	metric, ok := m.egress[id]
	if !ok {
		metric = &Metric{}
		m.egress[id] = metric
	}
	metric.RequestTime += d.Seconds()
	metric.RequestCount++
}

func (m *Metrics) GetEgress(id string) Metric {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if metric, ok := m.egress[id]; ok {
		return *metric
	}
	return Metric{}
}

func NewMetrics() *Metrics {
	return &Metrics{
		ingress: make(map[string]*Metric),
		egress:  make(map[string]*Metric),
	}
}
