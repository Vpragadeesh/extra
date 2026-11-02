package manager

import (
	"time"

	"torrent-downloader/network"
)

// EventType identifies the type of manager event
type EventType int

const (
	EventMetadataWaiting EventType = iota
	EventMetadataReceived
	EventProgress
	EventComplete
	EventError
)

// ManagerEvent represents an event emitted by the download manager
type ManagerEvent struct {
	Type      EventType
	TorrentID int
	Message   string
	Snapshot  ProgressSnapshot
	Err       error
}

// ProgressSnapshot contains a snapshot of the current download statistics
type ProgressSnapshot struct {
	Name              string
	TotalBytes        int64
	CompletedBytes    int64
	DownloadSpeed     float64 // bytes/sec
	UploadSpeed       float64 // bytes/sec
	ActivePeers       int
	TotalPeers        int
	PendingPeers      int
	HalfOpenPeers     int
	ConnectedSeeders  int
	PiecesComplete    int
	TotalPieces       int
	PiecesDirtiedGood int64
	PiecesDirtiedBad  int64
	ChunksWasted      int64
	ETA               time.Duration

	// Per-network breakdown (populated when multi-network is enabled)
	Networks []network.NetworkProgress
}
