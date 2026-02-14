package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"

	"github.com/quic-go/quic-go"
)

const TARGET = "127.0.0.1:496"

func main() {
	cert, _ := tls.LoadX509KeyPair("cert.pem", "key.pem")

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"quic-tunnel"},
	}

	listener, err := quic.ListenAddr(":9003", tlsConf, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("QUIC server listening on :9003")

	for {
		sess, _ := listener.Accept(context.Background())
		go handleSession(sess)
	}
}

func handleSession(sess quic.Connection) {
	for {
		stream, err := sess.AcceptStream(context.Background())
		if err != nil {
			return
		}
		go handleStream(stream)
	}
}

func handleStream(stream quic.Stream) {
	dst, err := net.Dial("tcp", TARGET)
	if err != nil {
		stream.Close()
		return
	}

	go io.Copy(dst, stream)
	go io.Copy(stream, dst)
}
