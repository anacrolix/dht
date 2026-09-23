package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
)

var bootstrapTestSequence atomic.Uint64

// A missing table file used to return after NewServer without Close. The deferred socket close
// then made serveUntilClosed panic: read on a closed connection while the server was still open.
func TestTableLoadErrorDoesNotPanic(t *testing.T) {
	oldFlags, oldServer := flags, s
	t.Cleanup(func() {
		flags = oldFlags
		s = oldServer
	})
	flags.TableFile = filepath.Join(t.TempDir(), "missing-table")
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = initServer(conn)
	if err == nil {
		t.Fatal("expected missing table file")
	}
	time.Sleep(300 * time.Millisecond)
}

func TestBootstrapJoinedBeforeSavingTable(t *testing.T) {
	oldFlags := flags
	oldServer := s
	oldArgs := os.Args
	oldMux := http.DefaultServeMux
	oldResolver := net.DefaultResolver
	oldBootstrapHosts := dht.DefaultGlobalBootstrapHostPorts
	defer func() {
		flags = oldFlags
		s = oldServer
		os.Args = oldArgs
		http.DefaultServeMux = oldMux
		net.DefaultResolver = oldResolver
		dht.DefaultGlobalBootstrapHostPorts = oldBootstrapHosts
	}()

	tableFile := filepath.Join(t.TempDir(), "nodes")
	if err := dht.WriteNodesToFile(nil, tableFile); err != nil {
		t.Fatal(err)
	}
	flags.Addr = "127.0.0.1:0"
	flags.TableFile = tableFile
	flags.NoBootstrap = false
	s = nil
	os.Args = []string{"dht-server"}
	http.DefaultServeMux = http.NewServeMux()
	// The DHT resolver caches failures across repeated tests in this process.
	dht.DefaultGlobalBootstrapHostPorts = []string{
		fmt.Sprintf("blocked-bootstrap-%d.invalid:6881", bootstrapTestSequence.Add(1)),
	}

	dnsStarted := make(chan struct{})
	releaseDNS := make(chan struct{})
	var started sync.Once
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			started.Do(func() { close(dnsStarted) })
			<-releaseDNS
			return nil, errors.New("injected DNS failure")
		},
	}

	done := make(chan error, 1)
	finished, released, signalSent := false, false, false
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !released {
			close(releaseDNS)
		}
		if !finished {
			if !signalSent {
				_ = proc.Signal(os.Interrupt)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("server did not stop during test cleanup")
			}
		}
	}()
	go func() { done <- mainErr() }()
	select {
	case <-dnsStarted:
	case <-time.After(time.Second):
		t.Fatal("bootstrap did not reach the controlled DNS lookup")
	}
	if err := os.Remove(tableFile); err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	signalSent = true
	select {
	case err := <-done:
		finished = true
		t.Fatalf("server returned before bootstrap completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := os.Stat(tableFile); !os.IsNotExist(err) {
		t.Errorf("table file was saved while bootstrap was still running: stat error %v", err)
	}
	close(releaseDNS)
	released = true
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatalf("mainErr(): %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not return after bootstrap was released")
	}
}
