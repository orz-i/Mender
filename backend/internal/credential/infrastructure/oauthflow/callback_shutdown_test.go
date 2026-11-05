package oauthflow

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type callbackPipeListener struct {
	connection net.Conn
	accepted   chan net.Conn
	once       sync.Once
	closed     chan struct{}
}

func (l *callbackPipeListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.accepted:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *callbackPipeListener) Addr() net.Addr { return l.connection.LocalAddr() }

func (l *callbackPipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func TestFlowFinishDrainsResponseAndHardCloseCanInterrupt(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(strconv.FormatBool(force), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(releaseHandler)
			body := strings.Repeat("complete callback response\n", 128)
			server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(started)
				<-release
				// Test HTTP framing without host HTML-injection middleware.
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				_, _ = io.WriteString(w, body)
			})}
			// Exercise the real net/http framing and server lifecycle over a
			// pipe, independently of OS loopback filters. TCP integration is
			// covered separately by the authorization flow tests.
			serverConnection, clientConnection := net.Pipe()
			listener := &callbackPipeListener{connection: serverConnection, accepted: make(chan net.Conn, 1), closed: make(chan struct{})}
			listener.accepted <- serverConnection
			serving := make(chan struct{})
			go func() { _ = server.Serve(listener); close(serving) }()
			f := &flow{ctx: t.Context(), now: time.Now, status: FlowFailed, listener: listener, server: server, urlReady: make(chan struct{})}
			t.Cleanup(func() {
				releaseHandler()
				f.shutdownListener()
				_ = clientConnection.Close()
				<-serving
			})
			type reply struct {
				body string
				err  error
			}
			received := make(chan reply, 1)
			transport := &http.Transport{DisableKeepAlives: true, DialContext: func(context.Context, string, string) (net.Conn, error) {
				return clientConnection, nil
			}}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			go func() {
				response, err := client.Get("http://callback.invalid/")
				if err != nil {
					received <- reply{err: err}
					return
				}
				defer func() { _ = response.Body.Close() }()
				data, err := io.ReadAll(response.Body)
				received <- reply{body: string(data), err: err}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("callback handler did not start")
			}
			finished := make(chan struct{})
			go func() { f.finish(); close(finished) }()
			select {
			case <-listener.closed:
			case <-time.After(5 * time.Second):
				t.Fatal("finish did not stop accepting new callbacks")
			}
			if force {
				forced := make(chan struct{})
				go func() { f.shutdownListener(); close(forced) }()
				select {
				case <-forced:
				case <-time.After(time.Second):
					t.Fatal("hard close waited for graceful callback drain")
				}
			}
			releaseHandler()
			select {
			case got := <-received:
				if !force && (got.err != nil || got.body != body) {
					t.Fatalf("normal finish truncated the active response: bytes=%d err=%v", len(got.body), got.err)
				}
				if force && got.err == nil {
					t.Fatal("hard close did not interrupt the active callback")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("callback request did not finish")
			}
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("callback drain did not finish")
			}
		})
	}
}
