package wt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"

	invoker "github.com/kixelated/invoker"
	quic "github.com/kixelated/quic-go"
	webtransport "github.com/kixelated/webtransport-go"
)

type Session struct {
	conn  quic.Connection
	inner *webtransport.Session

	inits   map[string]*MediaInit
	streams invoker.Tasks
}

func NewSession(connection quic.Connection, session *webtransport.Session) (s *Session, err error) {
	s = new(Session)
	s.conn = connection
	s.inner = session
	return s, nil
}

func (s *Session) Run(ctx context.Context) (err error) {
	return invoker.Run(ctx, s.runAccept, s.runAcceptUni, s.runInit, s.streams.Repeat)
}

func (s *Session) runAccept(ctx context.Context) (err error) {
	for {
		stream, err := s.inner.AcceptStream(ctx)
		if err != nil {
			return fmt.Errorf("failed to accept bidirectional stream: %w", err)
		}

		// Warp doesn't utilize bidirectional streams so just close them immediately.
		// We might use them in the future so don't close the connection with an error.
		stream.CancelRead(1)
	}
}

func (s *Session) runAcceptUni(ctx context.Context) (err error) {
	for {
		stream, err := s.inner.AcceptUniStream(ctx)
		if err != nil {
			return fmt.Errorf("failed to accept unidirectional stream: %w", err)
		}

		s.streams.Add(func(ctx context.Context) (err error) {
			return s.handleStream(ctx, stream)
		})
	}
}

func (s *Session) handleStream(ctx context.Context, stream webtransport.ReceiveStream) (err error) {
	defer func() {
		if err != nil {
			stream.CancelRead(1)
		}
	}()

	var header [8]byte
	for {
		_, err = io.ReadFull(stream, header[:])
		if errors.Is(io.EOF, err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to read atom header: %w", err)
		}

		size := binary.BigEndian.Uint32(header[0:4])
		name := string(header[4:8])

		if size < 8 {
			return fmt.Errorf("atom size is too small")
		} else if size > 42069 { // arbitrary limit
			return fmt.Errorf("atom size is too large")
		} else if name != "warp" {
			return fmt.Errorf("only warp atoms are supported")
		}

		payload := make([]byte, size-8)

		_, err = io.ReadFull(stream, payload)
		if err != nil {
			return fmt.Errorf("failed to read atom payload: %w", err)
		}

		log.Println("received message:", string(payload))

		msg := Message{}

		err = json.Unmarshal(payload, &msg)
		if err != nil {
			return fmt.Errorf("failed to decode json payload: %w", err)
		}

		if msg.Debug != nil {
			s.setDebug(msg.Debug)
		}
	}
}

func (s *Session) runInit(ctx context.Context) (err error) {
	for _, init := range s.inits {
		err = s.writeInit(ctx, init)
		if err != nil {
			return fmt.Errorf("failed to write init stream: %w", err)
		}
	}

	return nil
}

func (s *Session) writeInit(ctx context.Context, init *MediaInit) (err error) {
	temp, err := s.inner.OpenUniStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	if temp == nil {
		// Not sure when this happens, perhaps when closing a connection?
		return fmt.Errorf("received a nil stream from quic-go")
	}

	// Wrap the stream in an object that buffers writes instead of blocking.
	stream := NewStream(temp)
	s.streams.Add(stream.Run)

	defer func() {
		if err != nil {
			stream.WriteCancel(1)
		}
	}()

	stream.SetPriority(math.MaxInt)

	err = stream.WriteMessage(Message{
		Init: &MessageInit{Id: init.ID},
	})
	if err != nil {
		return fmt.Errorf("failed to write init header: %w", err)
	}

	_, err = stream.Write(init.Raw)
	if err != nil {
		return fmt.Errorf("failed to write init data: %w", err)
	}

	err = stream.Close()
	if err != nil {
		return fmt.Errorf("failed to close init stream: %w", err)
	}

	return nil
}

func (s *Session) setDebug(msg *MessageDebug) {
	s.conn.SetMaxBandwidth(uint64(msg.MaxBitrate))
}
