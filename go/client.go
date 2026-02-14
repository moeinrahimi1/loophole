package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type QuicClient struct {
	localAddr  string
	serverAddr string
	connection quic.Connection
}

func NewQuicClient(localAddr, serverAddr string) *QuicClient {
	return &QuicClient{
		localAddr:  localAddr,
		serverAddr: serverAddr,
	}
}

func (c *QuicClient) Start() error {
	// Connect to QUIC server
	if err := c.connectToServer(); err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}

	// Start local TCP listener
	listener, err := net.Listen("tcp", c.localAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", c.localAddr, err)
	}
	defer listener.Close()

	log.Printf("QUIC client listening on %s, forwarding to QUIC server at %s", c.localAddr, c.serverAddr)

	// Monitor connection health
	go c.keepAlive()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		log.Printf("new local connection from %s", conn.RemoteAddr())
		go c.handleConnection(conn)
	}
}

func (c *QuicClient) connectToServer() error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // In production, use proper certificate verification
		NextProtos:         []string{"quic-forwarder"},
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	conn, err := quic.DialAddr(context.Background(), c.serverAddr, tlsConfig, quicConfig)
	if err != nil {
		return err
	}

	c.connection = conn
	log.Printf("connected to QUIC server at %s", c.serverAddr)
	return nil
}

func (c *QuicClient) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.connection == nil {
			log.Println("connection lost, attempting to reconnect...")
			if err := c.connectToServer(); err != nil {
				log.Printf("reconnection failed: %v", err)
			}
		}
	}
}

func (c *QuicClient) handleConnection(localConn net.Conn) {
	defer localConn.Close()

	// Ensure we have a valid connection
	if c.connection == nil {
		log.Println("no active QUIC connection, attempting to reconnect...")
		if err := c.connectToServer(); err != nil {
			log.Printf("failed to reconnect: %v", err)
			return
		}
	}

	// Open a new stream for this connection
	stream, err := c.connection.OpenStreamSync(context.Background())
	if err != nil {
		log.Printf("failed to open stream: %v", err)
		// Try to reconnect
		c.connection = nil
		if err := c.connectToServer(); err != nil {
			log.Printf("reconnection failed: %v", err)
			return
		}
		// Retry opening stream
		stream, err = c.connection.OpenStreamSync(context.Background())
		if err != nil {
			log.Printf("failed to open stream after reconnect: %v", err)
			return
		}
	}
	defer stream.Close()

	log.Println("forwarding local connection through QUIC tunnel")

	// Bidirectional copy
	errChan := make(chan error, 2)

	// Copy from local connection to QUIC stream
	go func() {
		_, err := io.Copy(stream, localConn)
		errChan <- err
	}()

	// Copy from QUIC stream to local connection
	go func() {
		_, err := io.Copy(localConn, stream)
		errChan <- err
	}()

	// Wait for either direction to complete
	err = <-errChan
	if err != nil && err != io.EOF {
		log.Printf("forwarding error: %v", err)
	}
}

func main() {
	localAddr := flag.String("local", "127.0.0.1:8888", "Local listen address")
	serverAddr := flag.String("server", "localhost:4433", "QUIC server address")
	flag.Parse()

	client := NewQuicClient(*localAddr, *serverAddr)
	if err := client.Start(); err != nil {
		log.Fatalf("client error: %v", err)
	}
}