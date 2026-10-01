package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Call sends req to the daemon and streams the answer: stdout and stderr
// frames go to the given writers and data frames to onData (may be nil).
// stdin may be nil. It returns ErrNotRunning when no daemon answers.
func Call(ctx context.Context, dataDir string, req Request, stdin io.Reader, stdout, stderr io.Writer, onData func([]byte) error) (Exit, error) {
	return call(ctx, dataDir, req, stdin, stdout, stderr, onData, nil)
}

// Attach is Call for a terminal (KindAttach): it also sends each size that
// comes on resizes as a FrameResize.
func Attach(ctx context.Context, dataDir string, req Request, stdin io.Reader, stdout io.Writer, resizes <-chan [2]int) (Exit, error) {
	return call(ctx, dataDir, req, stdin, stdout, io.Discard, nil, resizes)
}

func call(ctx context.Context, dataDir string, req Request, stdin io.Reader, stdout, stderr io.Writer, onData func([]byte) error, resizes <-chan [2]int) (Exit, error) {
	secret, err := ReadSecret(dataDir)
	if err != nil {
		return Exit{}, ErrNotRunning
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	nc, err := d.DialContext(ctx, "unix", SocketPath(dataDir))
	if err != nil {
		return Exit{}, ErrNotRunning
	}
	defer nc.Close()
	stop := context.AfterFunc(ctx, func() { nc.Close() })
	defer stop()

	req.Secret = secret
	b, err := json.Marshal(req)
	if err != nil {
		return Exit{}, err
	}
	if err := WriteFrame(nc, FrameRequest, b); err != nil {
		return Exit{}, ErrNotRunning
	}
	var wmu sync.Mutex // stdin and resize frames are written from two goroutines
	go sendStdin(nc, &wmu, stdin)
	if resizes != nil {
		go func() {
			for size := range resizes {
				wmu.Lock()
				err := WriteFrame(nc, FrameResize, EncodeResize(size[0], size[1]))
				wmu.Unlock()
				if err != nil {
					return
				}
			}
		}()
	}

	for {
		typ, payload, err := ReadFrame(nc)
		if err != nil {
			if ctx.Err() != nil {
				return Exit{}, ctx.Err()
			}
			return Exit{}, fmt.Errorf("lost the connection to the ADBuddy host: %w", err)
		}
		switch typ {
		case FrameStdout:
			if _, err := stdout.Write(payload); err != nil {
				return Exit{}, err
			}
		case FrameStderr:
			if _, err := stderr.Write(payload); err != nil {
				return Exit{}, err
			}
		case FrameData:
			if onData != nil {
				if err := onData(payload); err != nil {
					return Exit{}, err
				}
			}
		case FrameExit:
			var e Exit
			if err := json.Unmarshal(payload, &e); err != nil {
				return Exit{}, err
			}
			return e, nil
		default:
			return Exit{}, errors.New("unexpected frame from the ADBuddy host")
		}
	}
}

func sendStdin(nc net.Conn, wmu *sync.Mutex, stdin io.Reader) {
	if stdin != nil {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				wmu.Lock()
				werr := WriteFrame(nc, FrameStdin, buf[:n])
				wmu.Unlock()
				if werr != nil {
					return
				}
			}
			if err != nil {
				break
			}
		}
	}
	wmu.Lock()
	_ = WriteFrame(nc, FrameStdinEOF, nil)
	wmu.Unlock()
}

// PingDaemon asks the daemon for its status.
func PingDaemon(ctx context.Context, dataDir string) (Ping, error) {
	var p Ping
	exit, err := Call(ctx, dataDir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, func(b []byte) error {
		return json.Unmarshal(b, &p)
	})
	if err != nil {
		return Ping{}, err
	}
	if exit.Code != 0 {
		return Ping{}, fmt.Errorf("ping failed: %s", exit.Error)
	}
	return p, nil
}
