package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type QuicServer struct {
	listenAddr  string
	forwardAddr string
	listener    *quic.Listener
}

func NewQuicServer(listenAddr, forwardAddr string) *QuicServer {
	return &QuicServer{
		listenAddr:  listenAddr,
		forwardAddr: forwardAddr,
	}
}

func (s *QuicServer) Start() error {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to generate TLS config: %v", err)
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	listener, err := quic.ListenAddr(s.listenAddr, tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	s.listener = listener
	log.Printf("QUIC server listening on %s, forwarding to %s", s.listenAddr, s.forwardAddr)

	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		log.Printf("new connection from %s", conn.RemoteAddr())
		go s.handleConnection(conn)
	}
}

func (s *QuicServer) handleConnection(conn quic.Connection) {
	defer conn.CloseWithError(0, "connection closed")

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Printf("failed to accept stream: %v", err)
			return
		}

		go s.handleStream(stream)
	}
}

func (s *QuicServer) handleStream(stream quic.Stream) {
	defer stream.Close()

	// Connect to the forward destination
	targetConn, err := net.DialTimeout("tcp", s.forwardAddr, 10*time.Second)
	if err != nil {
		log.Printf("failed to connect to forward address %s: %v", s.forwardAddr, err)
		return
	}
	defer targetConn.Close()

	log.Printf("forwarding stream to %s", s.forwardAddr)

	// Bidirectional copy
	errChan := make(chan error, 2)

	// Copy from QUIC stream to target
	go func() {
		_, err := io.Copy(targetConn, stream)
		errChan <- err
	}()

	// Copy from target to QUIC stream
	go func() {
		_, err := io.Copy(stream, targetConn)
		errChan <- err
	}()

	// Wait for either direction to complete
	err = <-errChan
	if err != nil && err != io.EOF {
		log.Printf("forwarding error: %v", err)
	}
}

func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-forwarder"},
	}, nil
}

func main() {
	listenAddr := flag.String("listen", "0.0.0.0:4433", "QUIC server listen address")
	forwardAddr := flag.String("forward", "localhost:8080", "Forward destination address")
	flag.Parse()

	server := NewQuicServer(*listenAddr, *forwardAddr)
	if err := server.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}