//go:build windows || unix

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V28: inbox/ is the agent's to replace at any moment. What comes from the
// phone is never written, moved nor deleted out of it through a link put in
// its place, before or after the host checks it.

// tree is what there is under dir, without following links: the size and
// hash of each regular file, "dir" for a folder and "link" for anything else.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		switch {
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[rel] = fmt.Sprintf("%d bytes %.8s", len(data), sumOf(data))
		case d.IsDir():
			out[rel] = "dir"
		default:
			out[rel] = "link"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// outsideDir is a folder out of the environment with files whose names an
// upload could take (an id, its .json), and whether links can be made here.
func outsideDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe")
	if err := dirLink(probe, t.TempDir()); err != nil {
		t.Skipf("no links to folders here: %v", err)
	}
	os.Remove(probe)
	os.Mkdir(filepath.Join(dir, ".partial"), 0o700)
	for _, n := range []string{"precious.txt", filepath.Join(".partial", fileID(2)), filepath.Join(".partial", fileID(2)+".json")} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("do not touch"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// swapInbox does what an agent can: inbox/ goes to inbox.real and a link to
// outside takes its name. The test removes the links it leaves (os.Remove:
// the link only) before the temporary folders are walked.
func swapInbox(ws, outside string) error {
	inbox := filepath.Join(ws, "inbox")
	if err := os.Rename(inbox, filepath.Join(ws, "inbox.real")); err != nil {
		return err
	}
	return dirLink(inbox, outside)
}

func removeLinks(t *testing.T, links ...string) {
	t.Cleanup(func() {
		for _, l := range links {
			if st, err := os.Lstat(l); err == nil && st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				os.Remove(l)
			}
		}
	})
}

// The swap happens right after the host checked inbox/ (opened false) or
// once it holds it open (opened true), for each thing that writes there: an
// upload that begins (.partial/<id>), one that ends (the move to its final
// name) and a screenshot. Nothing changes outside; the swap before the open
// fails, and after it what is written goes to the real folder.
func TestInboxSwappedAfterTheCheckIsContained(t *testing.T) {
	for _, op := range []string{"begin", "end", "capture"} {
		for _, opened := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s", op, map[bool]string{false: "checked", true: "opened"}[opened]), func(t *testing.T) {
				// The hook is set before the link starts its goroutines; what
				// it does is published through an atomic.
				var armed atomic.Pointer[func(bool)]
				setVar(t, &inboxHook, func(o bool) {
					if f := armed.Load(); f != nil {
						(*f)(o)
					}
				})
				r := newFilesRig(t, shotOf(pngData(20, 20)))
				outside := outsideDir(t)
				removeLinks(t, filepath.Join(r.ws, "inbox"), filepath.Join(outside, ".partial"))
				data := testBytes(20)
				id := fileID(2)
				if op == "end" {
					r.begin(t, "r_0", id, "a.bin", data)
					wantReady(t, readApp(t, r.c), "r_0", 0)
					sendData(t, r.c, id, data, 0, 20)
					// outside/.partial leads back to the real one: through the
					// links the source of the move is found.
					os.RemoveAll(filepath.Join(outside, ".partial"))
				}
				before := tree(t, outside)
				swapped := make(chan error, 1)
				var once sync.Once
				fn := func(o bool) {
					if o != opened {
						return
					}
					once.Do(func() {
						err := swapInbox(r.ws, outside)
						if err == nil && op == "end" {
							err = dirLink(filepath.Join(outside, ".partial"), filepath.Join(r.ws, "inbox.real", partialDir))
						}
						swapped <- err
					})
				}
				armed.Store(&fn)

				switch op {
				case "begin":
					r.begin(t, "r_1", id, "a.bin", data)
				case "end":
					sendApp(t, r.c, bridge.TypeAttachEnd, "r_1", bridge.AttachEnd{ID: id})
				case "capture":
					sendApp(t, r.c, bridge.TypeCaptureReq, "r_1", bridge.Empty{})
				}
				m := readApp(t, r.c)
				select {
				case err := <-swapped:
					if err != nil && op == "end" && runtime.GOOS == "windows" {
						// The written file is held open until it is filed, and
						// Windows renames no folder with an open file: nothing
						// can be swapped there.
						t.Skip(err)
					}
					if err != nil {
						t.Fatalf("swap: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("the host never reached the hook")
				}
				want := maps.Clone(before)
				if op == "end" {
					want[".partial"] = "link"
				}
				if got := tree(t, outside); !maps.Equal(got, want) {
					t.Fatalf("outside changed: %v, was %v (reply %s %s)", got, want, m.Type, m.Payload)
				}
				if !opened {
					wantFileError(t, m, "r_1", bridge.CodeFailed)
					return
				}
				// Held open, inbox/ is the folder itself: what is written goes
				// to it, under its new name.
				real := tree(t, filepath.Join(r.ws, "inbox.real"))
				switch op {
				case "begin":
					wantReady(t, m, "r_1", 0)
					if _, ok := real[filepath.Join(partialDir, id)]; !ok {
						t.Fatalf("no partial in the real inbox: %v", real)
					}
				case "end":
					if a := wantDone(t, m, "r_1"); a.SHA256 != sumOf(data) {
						t.Fatalf("%+v", a)
					}
					if len(real) != 2 { // .partial and the file
						t.Fatalf("real inbox: %v", real)
					}
				case "capture":
					wantType(t, m, "r_1", bridge.TypeCaptureReady)
					if len(real) != 2 {
						t.Fatalf("real inbox: %v", real)
					}
				}
			})
		}
	}
}

// inbox/ became a link before attach.end, and outside/.partial leads back to
// the real .partial: endUpload refuses, and its failure removes nothing
// through the links (by path it removed the real upload through them).
func TestEndUploadChecksTheInbox(t *testing.T) {
	r := newFilesRig(t, nil)
	outside := outsideDir(t)
	removeLinks(t, filepath.Join(r.ws, "inbox"), filepath.Join(outside, ".partial"))
	data := testBytes(20)
	id := fileID(3)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data, 0, 20)
	r.l.files.upMu.Lock()
	r.l.files.uploads[id].f.Close() // Windows renames no folder with an open file
	r.l.files.upMu.Unlock()
	os.RemoveAll(filepath.Join(outside, ".partial"))
	if err := swapInbox(r.ws, outside); err != nil {
		t.Skip(err)
	}
	if err := dirLink(filepath.Join(outside, ".partial"), filepath.Join(r.ws, "inbox.real", partialDir)); err != nil {
		t.Fatal(err)
	}
	before := tree(t, outside)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeFailed)
	if got := tree(t, outside); !maps.Equal(got, before) {
		t.Fatalf("outside changed: %v, was %v", got, before)
	}
	if _, err := os.Lstat(filepath.Join(r.ws, "inbox.real", partialDir, id)); err != nil {
		t.Fatalf("the upload was lost: %v", err)
	}
}

// .partial/<id> replaced by another file after the upload was written (the
// hard link would take whatever is there): the upload fails and nothing is
// filed, neither that file nor a link.
func TestEndUploadFilesOnlyTheFileWritten(t *testing.T) {
	var armed atomic.Pointer[func(bool)]
	setVar(t, &inboxHook, func(o bool) {
		if f := armed.Load(); f != nil {
			(*f)(o)
		}
	})
	r := newFilesRig(t, nil)
	data := testBytes(20)
	id := fileID(2)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data, 0, 20)
	replaced := make(chan error, 1)
	var once sync.Once
	fn := func(opened bool) {
		if !opened {
			return
		}
		once.Do(func() {
			p := filepath.Join(r.ws, "inbox", partialDir, id)
			err := os.Remove(p)
			if err == nil {
				err = os.WriteFile(p, []byte("another file"), 0o600)
			}
			replaced <- err
		})
	}
	armed.Store(&fn)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	wantFileError(t, readApp(t, r.c), "r_2", bridge.CodeFailed)
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
	if files := r.inboxFiles(t); len(files) != 0 {
		t.Fatalf("filed: %v", files)
	}
}

// The written file's descriptor is still open when it is filed: its inode
// cannot be reused by a file put in its place (Linux does it at once).
func TestEndUploadKeepsTheWrittenFileOpenWhileFiling(t *testing.T) {
	var openErr error
	called := false
	setVar(t, &moveHook, func(f *os.File) {
		called = true
		_, openErr = f.Stat() // fails with os.ErrClosed once closed
	})
	r := newFilesRig(t, nil)
	data := testBytes(20)
	id := fileID(2)
	r.begin(t, "r_1", id, "a.bin", data)
	wantReady(t, readApp(t, r.c), "r_1", 0)
	sendData(t, r.c, id, data, 0, 20)
	sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
	readApp(t, r.c)
	if !called {
		t.Fatal("moveHook was not called")
	}
	if openErr != nil {
		t.Fatalf("the written file was closed before filing: %v", openErr)
	}
}

// A failed upload removes its .partial/<id>, never one through a link put in
// place of inbox/: at attach.end, and when a frame carries more than declared.
func TestPartialIsNeverRemovedThroughALink(t *testing.T) {
	for _, when := range []string{"end", "frame"} {
		t.Run(when, func(t *testing.T) {
			r := newFilesRig(t, nil)
			outside := outsideDir(t)
			removeLinks(t, filepath.Join(r.ws, "inbox"))
			data := testBytes(20)
			id := fileID(2) // the name of the files outside/.partial has
			r.begin(t, "r_1", id, "a.bin", data)
			wantReady(t, readApp(t, r.c), "r_1", 0)
			r.l.files.upMu.Lock()
			r.l.files.uploads[id].f.Close()
			r.l.files.upMu.Unlock()
			if err := swapInbox(r.ws, outside); err != nil {
				t.Skip(err)
			}
			before := tree(t, outside)
			if when == "frame" {
				sendFrame(t, r.c, id, 0, testBytes(21))
			}
			sendApp(t, r.c, bridge.TypeAttachEnd, "r_2", bridge.AttachEnd{ID: id})
			wantType(t, readApp(t, r.c), "r_2", bridge.TypeError)
			if got := tree(t, outside); !maps.Equal(got, before) {
				t.Fatalf("outside changed: %v, was %v", got, before)
			}
		})
	}
}

// At most maxDownloads send at once and maxPendingDownloads are asked per
// connection: one more is busy, and the others wait their turn instead of
// failing (the app asks for every image of the chat at once).
func TestDownloadsAreCapped(t *testing.T) {
	r := newFilesRig(t, nil)
	r.bigStored(t, fileID(1))
	for i := 2; i <= maxPendingDownloads+1; i++ {
		r.l.chat.attachments.put(bridge.Attachment{ID: fileID(i), Name: "big.bin", Size: 64 << 20}, "outbox/big.bin")
	}
	for i := 1; i <= maxPendingDownloads; i++ {
		sendApp(t, r.c, bridge.TypeFileGet, fmt.Sprintf("r_%d", i), bridge.FileGet{ID: fileID(i)})
	}
	waitUntil(t, "the downloads", func() bool { return r.downloadsRunning() == maxPendingDownloads })
	sendApp(t, r.c, bridge.TypeFileGet, "r_over", bridge.FileGet{ID: fileID(maxPendingDownloads + 1)})
	ids := map[string]bool{}
	busy := false
	for frames := 0; frames < 300; {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		typ, data, err := r.c.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("after %d frames: %v", frames, err)
		}
		if typ == websocket.MessageBinary {
			fid, _, _, err := bridge.DecodeFrame(data)
			if err != nil {
				t.Fatal(err)
			}
			ids[fid.String()] = true
			frames++
			continue
		}
		var m bridge.Message
		json.Unmarshal(data, &m)
		if busy {
			t.Fatalf("another message: %s %s", m.Type, m.Payload)
		}
		wantFileError(t, m, "r_over", bridge.CodeBusy)
		busy = true
	}
	if !busy {
		t.Fatal("no busy for one download over the cap")
	}
	if len(ids) > maxDownloads {
		t.Fatalf("%d downloads sent at once, at most %d", len(ids), maxDownloads)
	}
	if n := r.downloadsRunning(); n != maxPendingDownloads {
		t.Fatalf("%d downloads kept, want %d", n, maxPendingDownloads)
	}
}
