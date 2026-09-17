package mesh

import (
	"context"
	"encoding/json"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/asynchronomatic/speakeasy/pkg/log"
)

const topicName = "events"

type EventManager struct {
	h     host.Host
	ps    *pubsub.PubSub
	topic *pubsub.Topic
	sub   *pubsub.Subscription
	c     chan PeerEvent
}

func (em *EventManager) ForceUpdate() error {
	ev := &PeerEvent{
		PeerID: em.h.ID().String(),
		Status: PeerStatusSync,
	}

	data, err := json.Marshal(&ev)
	if err != nil {
		return err
	}
	return em.topic.Publish(context.Background(), data)
}

func (em *EventManager) Serve(ctx context.Context) error {
	for {
		msg, err := em.sub.Next(ctx)
		if err != nil {
			continue
		}
		if msg.ReceivedFrom == em.h.ID() {
			continue // skip our own publishes
		}

		ev := PeerEvent{}
		err = json.Unmarshal(msg.Data, &ev)
		if err != nil {
			continue
		}

		select {
		case em.c <- ev:
		default:
			log.WithName("evt").Eventf("event manager channel full")
		}

	}
}

func (em *EventManager) Out() <-chan PeerEvent {
	return em.c
}

func NewEventManager(h host.Host) (*EventManager, error) {
	ps, err := pubsub.NewGossipSub(context.Background(), h)
	if err != nil {
		return nil, err
	}

	topic, err := ps.Join(topicName)
	if err != nil {
		return nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	em := &EventManager{
		h:     h,
		ps:    ps,
		topic: topic,
		sub:   sub,
		c:     make(chan PeerEvent, 64),
	}

	// FIXME:
	go em.Serve(context.Background())

	return em, nil
}
