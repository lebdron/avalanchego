// Copyright (C) 2019-2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gossip

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network/p2p"
	"github.com/ava-labs/avalanchego/utils/bloom"
	"github.com/ava-labs/avalanchego/utils/logging"
)

var _ p2p.Handler = (*Handler[*testTx])(nil)

func NewHandler[T Gossipable](
	log logging.Logger,
	marshaller Marshaller[T],
	set Set[T],
	metrics Metrics,
	targetResponseSize int,
) *Handler[T] {
	return &Handler[T]{
		Handler:            p2p.NoOpHandler{},
		log:                log,
		marshaller:         marshaller,
		set:                set,
		metrics:            metrics,
		targetResponseSize: targetResponseSize,
	}
}

type Handler[T Gossipable] struct {
	p2p.Handler
	marshaller         Marshaller[T]
	log                logging.Logger
	set                Set[T]
	metrics            Metrics
	targetResponseSize int
}

func (h Handler[T]) AppRequest(_ context.Context, nodeID ids.NodeID, _ time.Time, requestBytes []byte) ([]byte, error) {
	h.log.Debug("Handler::AppRequest", zap.Stringer("nodeID", nodeID))
	filter, salt, err := ParseAppRequest(requestBytes)
	if err != nil {
		return nil, err
	}

	responseSize := 0
	gossipBytes := make([][]byte, 0)
	h.set.Iterate(func(gossipable T) bool {
		gossipID := gossipable.GossipID()

		// filter out what the requesting peer already knows about
		if bloom.Contains(filter, gossipID[:], salt[:]) {
			return true
		}

		var bytes []byte
		bytes, err = h.marshaller.MarshalGossip(gossipable)
		if err != nil {
			h.log.Debug("failed to marshal gossip", zap.Error(err))
			return false
		}

		// check that this doesn't exceed our maximum configured target response
		// size
		gossipBytes = append(gossipBytes, bytes)
		responseSize += len(bytes)

		return responseSize <= h.targetResponseSize
	})

	if err != nil {
		return nil, err
	}

	sentCountMetric, err := h.metrics.sentCount.GetMetricWith(pullLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to get sent count metric: %w", err)
	}

	sentBytesMetric, err := h.metrics.sentBytes.GetMetricWith(pullLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to get sent bytes metric: %w", err)
	}

	sentCountMetric.Add(float64(len(gossipBytes)))
	sentBytesMetric.Add(float64(responseSize))

	h.log.Debug("Handler::AppRequest",
		zap.Stringer("nodeID", nodeID),
		zap.Int("len(gossipBytes)", len(gossipBytes)),
		zap.Int("responseSize", responseSize),
	)

	return MarshalAppResponse(gossipBytes)
}

func (h Handler[T]) AppGossip(_ context.Context, nodeID ids.NodeID, gossipBytes []byte) {
	h.log.Debug("Handler::AppGossip", zap.Stringer("nodeID", nodeID))
	gossip, err := ParseAppGossip(gossipBytes)
	if err != nil {
		h.log.Debug("failed to unmarshal gossip", zap.Error(err))
		return
	}

	receivedBytes := 0
	gossipables := make([]T, 0, len(gossip))
	for _, bytes := range gossip {
		receivedBytes += len(bytes)
		gossipable, err := h.marshaller.UnmarshalGossip(bytes)
		if err != nil {
			h.log.Debug("failed to unmarshal gossip",
				zap.Stringer("nodeID", nodeID),
				zap.Error(err),
			)
			continue
		}

		gossipables = append(gossipables, gossipable)
	}

	errs := h.set.Add(gossipables...)
	for i, err := range errs {
		if err != nil {
			h.log.Debug(
				"failed to add gossip to the known set",
				zap.Stringer("nodeID", nodeID),
				zap.Stringer("id", gossipables[i].GossipID()),
				zap.Error(err),
			)
		}
	}

	receivedCountMetric, err := h.metrics.receivedCount.GetMetricWith(pushLabels)
	if err != nil {
		h.log.Error("failed to get received count metric", zap.Error(err))
		return
	}

	receivedBytesMetric, err := h.metrics.receivedBytes.GetMetricWith(pushLabels)
	if err != nil {
		h.log.Error("failed to get received bytes metric", zap.Error(err))
		return
	}

	receivedCountMetric.Add(float64(len(gossip)))
	receivedBytesMetric.Add(float64(receivedBytes))

	h.log.Debug("Handler::AppGossip",
		zap.Stringer("nodeID", nodeID),
		zap.Int("len(gossip)", len(gossip)),
		zap.Int("receivedBytes", receivedBytes),
	)
}
