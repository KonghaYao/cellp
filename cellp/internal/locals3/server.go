package locals3

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3bolt"
	"go.etcd.io/bbolt"
)

// Server is an in-process path-style S3 for `cellp dev` (no Docker / RustFS).
type Server struct {
	Addr     string
	listener net.Listener
	http     *http.Server
	db       *bbolt.DB
}

// Start opens a Bolt-backed fake S3 on addr (e.g. 127.0.0.1:19000).
func Start(addr, boltPath string) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(boltPath), 0o755); err != nil {
		return nil, err
	}
	db, err := bbolt.Open(boltPath, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open s3 bolt: %w", err)
	}
	backend := s3bolt.New(db)
	for _, b := range []string{"cellp-celld", "cellp-artifacts", "cellp-offshoot"} {
		if err := backend.CreateBucket(b); err != nil {
			// already exists is fine
			_ = err
		}
	}
	faker := gofakes3.New(backend, gofakes3.WithAutoBucket(true))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	hs := &http.Server{Handler: faker.Server()}
	s := &Server{Addr: "http://" + ln.Addr().String(), listener: ln, http: hs, db: db}
	go func() { _ = hs.Serve(ln) }()
	return s, nil
}

// Close stops the listener and Bolt DB.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.http != nil {
		err = s.http.Close()
	}
	if s.db != nil {
		if cerr := s.db.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
