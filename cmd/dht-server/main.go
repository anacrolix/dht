package main

import (
	"context"
	"errors"
	stdLog "log"
	"net"
	"net/http"
	"os"
	"os/signal"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/log"
	"github.com/anacrolix/tagflag"

	"github.com/anacrolix/dht/v2"
)

var (
	flags = struct {
		TableFile   string `help:"name of file for storing node info"`
		Addr        string `help:"local UDP address"`
		NoBootstrap bool
	}{
		Addr: ":0",
	}
	s *dht.Server
)

func loadTable() (err error) {
	added, err := s.AddNodesFromFile(flags.TableFile)
	log.Printf("loaded %d nodes from table file", added)
	return
}

// initServer starts the node on conn and loads flags.TableFile when set. A load error closes the
// server before returning, so the caller's deferred conn.Close cannot race an open serve loop.
func initServer(conn net.PacketConn) error {
	cfg := dht.NewDefaultServerConfig()
	cfg.Conn = conn
	cfg.Logger = log.Default.FilterLevel(log.Info)
	cfg.NoSecurity = false
	var err error
	s, err = dht.NewServer(cfg)
	if err != nil {
		return err
	}
	if flags.TableFile == "" {
		return nil
	}
	if err = loadTable(); err != nil {
		s.Close()
		return err
	}
	return nil
}

func saveTable() error {
	return dht.WriteNodesToFile(s.Nodes(), flags.TableFile)
}

func main() {
	stdLog.SetFlags(stdLog.LstdFlags | stdLog.Lshortfile)
	err := mainErr()
	if err != nil {
		log.Printf("error in main: %v", err)
		os.Exit(1)
	}
}

func mainErr() error {
	tagflag.Parse(&flags)
	conn, err := net.ListenPacket("udp", flags.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = initServer(conn); err != nil {
		return err
	}
	defer s.Close()
	http.HandleFunc("/debug/dht", func(w http.ResponseWriter, r *http.Request) {
		s.WriteStatus(w)
	})
	log.Printf("dht server on %s, ID is %x", s.Addr(), s.ID())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var bootstrapDone <-chan struct{}
	if !flags.NoBootstrap {
		done := make(chan struct{})
		bootstrapDone = done
		go func() {
			defer close(done)
			tried, err := s.BootstrapContext(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("error bootstrapping: %s", err)
				}
			} else {
				log.Printf("finished bootstrapping: %#v", tried)
			}
		}()
	}
	<-ctx.Done()
	s.Close()
	if bootstrapDone != nil {
		<-bootstrapDone
	}

	if flags.TableFile != "" {
		if err := saveTable(); err != nil {
			log.Printf("error saving node table: %s", err)
		}
	}
	return nil
}
