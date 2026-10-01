package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// Files between the phone and the PC (spec M3 §6, protocol spec §6): uploads
// to inbox/, downloads of what the registry knows and screenshots. Neither
// the names nor the contents of the files go to any log or error.

// Limits of the files (spec M3 §6.1, protocol spec §9).
const (
	maxUploadSize   = 200 << 20 // per file
	maxUploads      = 4         // in progress per connection
	maxMIME         = 255
	maxCaptureBytes = 64 << 20
	partialDir      = ".partial" // inbox/.partial/<id> and <id>.json
)

var (
	// inboxQuota is what inbox/ may weigh, partial uploads included.
	inboxQuota int64 = 2 << 30
	// uploadStale is how long an upload may go without advancing.
	uploadStale = time.Hour
	// captureTimeout is how long adb has to take the screenshot.
	captureTimeout = 10 * time.Second
	// filesNow is the clock of the files, a variable for the tests.
	filesNow = time.Now
)

// filesState is what the files keep. Two mutexes, neither held while calling
// the Manager or the bridge server (the handlers answer after releasing it):
// upMu guards the uploads, and every frame of the app takes it too; dlMu
// guards the downloads and the capture flag.
type filesState struct {
	upMu    sync.Mutex
	uploads map[string]*upload // by id, of this connection only

	dlMu      sync.Mutex
	downloads map[string]*download
	capturing bool
}

// upload is an upload of this connection. A finished one stays (done) so that
// a retry after a lost attach.done gets it again.
type upload struct {
	sha, name, mime string
	size            int64
	f               *os.File // inbox/.partial/<id>; nil once dropped
	written         int64
	h               hash.Hash
	last            time.Time
	bad             bool               // dropped: more data than it declared
	broken          bool               // dropped: writing to disk failed
	ready           bool               // h covers what was already there; frames wait for it
	done            *bridge.Attachment // finished
}

type download struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// partialMeta is inbox/.partial/<id>.json: what a resumption must match.
type partialMeta struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// useWorkspace gives the chat the environment: inbox/ and outbox/ hang from it.
func (l *phoneLink) useWorkspace(ws string) {
	l.mu.Lock()
	l.ws = ws
	l.mu.Unlock()
}

func (l *phoneLink) workspaceAndADB() (string, phoneapp.ADB) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ws, l.adb
}

// filesHandlers are the handlers chatArea registers.
func (l *phoneLink) filesHandlers() map[string]func(bridge.Message) {
	return map[string]func(bridge.Message){
		bridge.TypeAttachBegin: l.onAttachBegin,
		bridge.TypeAttachEnd:   l.onAttachEnd,
		bridge.TypeFileGet:     l.onFileGet,
		bridge.TypeFileCancel:  l.onFileCancel,
		bridge.TypeCaptureReq:  l.onCaptureRequest,
	}
}

// resetFiles starts what is kept per connection over (a new connection or
// none): the uploads are forgotten (their .partial stays on disk, to resume)
// and the downloads stop.
func (l *phoneLink) resetFiles() {
	l.files.upMu.Lock()
	for _, u := range l.files.uploads {
		if u.f != nil {
			u.f.Close()
		}
	}
	l.files.uploads = nil
	l.files.upMu.Unlock()
	l.files.dlMu.Lock()
	for _, d := range l.files.downloads {
		d.cancel()
	}
	l.files.downloads = nil
	l.files.dlMu.Unlock()
}

func (l *phoneLink) current(m bridge.Message) bool { return l.srv != nil && l.srv.Current(m) }

func validSHA(s string) (string, bool) {
	if len(s) != 64 {
		return "", false
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(s), true
}

// thumbnailOf makes the thumbnail of a file, a variable for the tests.
var thumbnailOf = thumbnail

// ---- uploads ----

// onAttachBegin starts or resumes an upload.
func (l *phoneLink) onAttachBegin(m bridge.Message) {
	if !l.current(m) {
		return
	}
	var ab bridge.AttachBegin
	if json.Unmarshal(m.Payload, &ab) != nil {
		l.fail(m, bridge.CodeInvalid, "The request is not valid.")
		return
	}
	_, err := bridge.ParseFrameID(ab.ID)
	sha, shaOK := validSHA(ab.SHA256)
	switch {
	case err != nil:
		l.fail(m, bridge.CodeInvalid, "The attachment has no valid id.")
		return
	case !shaOK:
		l.fail(m, bridge.CodeInvalid, "The sha256 is not 64 hexadecimal digits.")
		return
	case ab.Size < 0 || len(ab.MIME) > maxMIME:
		l.fail(m, bridge.CodeInvalid, "The size or the type of the file is not valid.")
		return
	case ab.Size > maxUploadSize:
		l.fail(m, bridge.CodeTooLarge, "The file is too large: at most 200 MiB.")
		return
	}
	ab.SHA256 = sha
	offset, code, text := l.beginUpload(ab)
	if code != "" {
		l.fail(m, code, text)
		return
	}
	_ = l.srv.Reply(m, bridge.TypeAttachReady, bridge.AttachReady{ID: ab.ID, Offset: offset})
}

// beginUpload does the work of attach.begin and returns the offset to go on
// from, or an error code and its text.
func (l *phoneLink) beginUpload(ab bridge.AttachBegin) (int64, string, string) {
	fs := &l.files
	fs.upMu.Lock()
	u, offset, code, text := l.beginUploadLocked(ab)
	fs.upMu.Unlock()
	if code != "" || u == nil || u.ready {
		return offset, code, text
	}
	// A resumption hashes what is already there once, without the mutex (it
	// can be 200 MiB); the entry takes no frames until it is ready.
	h := sha256.New()
	_, err := io.Copy(h, io.NewSectionReader(u.f, 0, offset))
	fs.upMu.Lock()
	defer fs.upMu.Unlock()
	if err != nil {
		if fs.uploads[ab.ID] == u {
			delete(fs.uploads, ab.ID)
			u.f.Close()
		}
		return 0, bridge.CodeFailed, "Could not read what was already uploaded."
	}
	if fs.uploads[ab.ID] != u {
		// Another connection took over meanwhile.
		return 0, bridge.CodeFailed, "The upload was interrupted."
	}
	u.h = h
	u.ready = true
	return offset, "", ""
}

// beginUploadLocked is the part of beginUpload that runs with upMu held. It
// returns the new upload, or nil when it answers by itself (an error, or a
// retry of a finished one).
func (l *phoneLink) beginUploadLocked(ab bridge.AttachBegin) (*upload, int64, string, string) {
	ws, _ := l.workspaceAndADB()
	if ws == "" {
		return nil, 0, bridge.CodeFailed, "The host has no environment to keep files in."
	}
	fs := &l.files
	if fs.uploads == nil {
		fs.uploads = map[string]*upload{}
	}
	now := filesNow()

	// Finished before: the app lost the answer and asks again.
	if a, ok := l.chat.attachments.get(ab.ID); ok {
		if a.SHA256 != ab.SHA256 {
			return nil, 0, bridge.CodeInvalid, "An attachment with this id already exists with other content."
		}
		if old := fs.uploads[ab.ID]; old != nil && old.f != nil {
			old.f.Close()
		}
		att := a.Attachment
		fs.uploads[ab.ID] = &upload{sha: ab.SHA256, size: a.Size, last: now, done: &att}
		return nil, a.Size, "", ""
	}

	inbox := filepath.Join(ws, "inbox")
	partials := filepath.Join(inbox, partialDir)
	if err := os.MkdirAll(partials, 0o700); err != nil {
		return nil, 0, bridge.CodeFailed, "Could not create the inbox folder."
	}
	l.dropStaleLocked(partials, now)
	if old := fs.uploads[ab.ID]; old != nil {
		// What it wrote stays in .partial: the resumption below takes it.
		if old.f != nil {
			old.f.Close()
		}
		delete(fs.uploads, ab.ID)
	}
	running := 0
	for _, u := range fs.uploads {
		if u.done == nil {
			running++
		}
	}
	if running >= maxUploads {
		return nil, 0, bridge.CodeBusy, "Too many uploads at once: wait for one to finish."
	}
	if inboxUsedLocked(inbox, ab.ID, fs.uploads)+ab.Size > inboxQuota {
		return nil, 0, bridge.CodeTooLarge, "The inbox is full (2 GiB): free space by deleting files from the inbox folder of the environment."
	}

	data := filepath.Join(partials, ab.ID)
	meta := filepath.Join(partials, ab.ID+".json")
	f, offset, err := openPartial(data, meta, ab)
	if err != nil {
		return nil, 0, bridge.CodeFailed, "Could not keep the upload."
	}
	u := &upload{sha: ab.SHA256, name: ab.Name, mime: ab.MIME, size: ab.Size, f: f, written: offset, h: sha256.New(), last: now, ready: offset == 0}
	fs.uploads[ab.ID] = u
	return u, offset, "", ""
}

// openPartial opens inbox/.partial/<id> to write: the one a previous upload
// with the same hash left (and the offset it reached), or a new one. A
// .partial that is a link, a folder or too large is never followed: it is
// removed and the upload starts at 0.
func openPartial(data, meta string, ab bridge.AttachBegin) (*os.File, int64, error) {
	st, err := os.Lstat(data)
	switch {
	case err == nil && st.Mode().IsRegular() && st.Size() <= ab.Size && metaMatches(meta, ab):
		f, err := os.OpenFile(data, os.O_RDWR, 0o600)
		if err != nil {
			return nil, 0, err
		}
		if fst, err := f.Stat(); err != nil || !os.SameFile(st, fst) {
			f.Close()
			return nil, 0, errors.New("the partial file changed")
		}
		if err := writeMeta(meta, ab); err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, st.Size(), nil
	case err == nil:
		if err := os.Remove(data); err != nil {
			return nil, 0, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, 0, err
	}
	f, err := os.OpenFile(data, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, 0, err
	}
	if err := writeMeta(meta, ab); err != nil {
		f.Close()
		os.Remove(data)
		return nil, 0, err
	}
	return f, 0, nil
}

func metaMatches(meta string, ab bridge.AttachBegin) bool {
	st, err := os.Lstat(meta)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 4<<10 {
		return false
	}
	raw, err := os.ReadFile(meta)
	if err != nil {
		return false
	}
	var pm partialMeta
	return json.Unmarshal(raw, &pm) == nil && pm.SHA256 == ab.SHA256 && pm.Size == ab.Size
}

func writeMeta(meta string, ab bridge.AttachBegin) error {
	if st, err := os.Lstat(meta); err == nil && !st.Mode().IsRegular() {
		if err := os.Remove(meta); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(partialMeta{SHA256: ab.SHA256, Size: ab.Size})
	return os.WriteFile(meta, raw, 0o600)
}

// inboxUsedLocked is what inbox/ weighs for the quota: the files in it, the
// partial uploads left on disk and the declared size of those in progress
// (instead of what they wrote so far), except the one with id skip, which is
// being replaced. Call it with upMu held.
func inboxUsedLocked(inbox, skip string, uploads map[string]*upload) int64 {
	var total int64
	if entries, err := os.ReadDir(inbox); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
				total += info.Size()
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(inbox, partialDir)); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") || e.Name() == skip {
				continue
			}
			if u := uploads[e.Name()]; u != nil && u.done == nil {
				continue
			}
			if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
				total += info.Size()
			}
		}
	}
	for id, u := range uploads {
		if u.done == nil && id != skip {
			total += u.size
		}
	}
	return total
}

// dropStaleLocked deletes the uploads that did not advance for uploadStale,
// those in memory and those left on disk by an earlier connection.
func (l *phoneLink) dropStaleLocked(partials string, now time.Time) {
	for id, u := range l.files.uploads {
		if u.done == nil && now.Sub(u.last) > uploadStale {
			if u.f != nil {
				u.f.Close()
			}
			os.Remove(filepath.Join(partials, id))
			os.Remove(filepath.Join(partials, id+".json"))
			delete(l.files.uploads, id)
		}
	}
	entries, err := os.ReadDir(partials)
	if err != nil {
		return
	}
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if u := l.files.uploads[id]; u != nil {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > uploadStale {
			os.Remove(filepath.Join(partials, e.Name()))
		}
	}
}

// onUploadFrame receives a frame of an upload. It carries no rid: whatever is
// wrong with it is told at attach.end. Frames of an id that was not accepted
// on this connection (or of a connection already replaced, whose uploads
// were forgotten), or that do not start where the file stands, are dropped.
func (l *phoneLink) onUploadFrame(fid bridge.FrameID, offset int64, data []byte) {
	fs := &l.files
	fs.upMu.Lock()
	defer fs.upMu.Unlock()
	id := fid.String()
	u := fs.uploads[id]
	if u == nil || u.done != nil || u.bad || u.broken || !u.ready || u.f == nil || offset != u.written {
		return
	}
	if u.written+int64(len(data)) > u.size {
		// More than it declared: the upload is dropped, and attach.end says so.
		u.bad = true
		u.f.Close()
		u.f = nil
		if ws, _ := l.workspaceAndADB(); ws != "" {
			partials := filepath.Join(ws, "inbox", partialDir)
			os.Remove(filepath.Join(partials, id))
			os.Remove(filepath.Join(partials, id+".json"))
		}
		return
	}
	if _, err := u.f.WriteAt(data, offset); err != nil {
		u.broken = true
		u.f.Close()
		u.f = nil
		return
	}
	u.h.Write(data)
	u.written += int64(len(data))
	u.last = filesNow()
}

// onAttachEnd checks and files a finished upload.
func (l *phoneLink) onAttachEnd(m bridge.Message) {
	if !l.current(m) {
		return
	}
	var ae bridge.AttachEnd
	if json.Unmarshal(m.Payload, &ae) != nil {
		l.fail(m, bridge.CodeInvalid, "The request is not valid.")
		return
	}
	if _, err := bridge.ParseFrameID(ae.ID); err != nil {
		l.fail(m, bridge.CodeInvalid, "The attachment has no valid id.")
		return
	}
	att, final, code, text := l.endUpload(ae.ID)
	if code == "" && final != "" {
		// Outside the mutex: decoding a large image takes seconds.
		att.Thumb = thumbnailOf(filepath.Join(l.inboxDir(), final))
		l.finishUpload(att, final)
	}
	if code != "" {
		l.fail(m, code, text)
		return
	}
	_ = l.srv.Reply(m, bridge.TypeAttachDone, bridge.AttachDone{Attachment: att})
}

// inboxDir is inbox/ of the environment ("" without one).
func (l *phoneLink) inboxDir() string {
	ws, _ := l.workspaceAndADB()
	if ws == "" {
		return ""
	}
	return filepath.Join(ws, "inbox")
}

// endUpload checks a finished upload and moves it to its final name in
// inbox/. It returns the attachment (without thumb) and the final name; for a
// retry of a finished one the attachment is the existing one and the name "".
// The move (a copy where hard links are not possible) runs without upMu.
func (l *phoneLink) endUpload(id string) (bridge.Attachment, string, string, string) {
	ws, _ := l.workspaceAndADB()
	fs := &l.files
	fs.upMu.Lock()
	u := fs.uploads[id]
	if u == nil || ws == "" {
		fs.upMu.Unlock()
		return bridge.Attachment{}, "", bridge.CodeNotFound, "There is no upload with this id."
	}
	if u.done != nil {
		fs.upMu.Unlock()
		return *u.done, "", "", ""
	}
	inbox := filepath.Join(ws, "inbox")
	partials := filepath.Join(inbox, partialDir)
	fail := func(code, text string) (bridge.Attachment, string, string, string) {
		if u.f != nil {
			u.f.Close()
		}
		os.Remove(filepath.Join(partials, id))
		os.Remove(filepath.Join(partials, id+".json"))
		delete(fs.uploads, id)
		fs.upMu.Unlock()
		return bridge.Attachment{}, "", code, text
	}
	switch {
	case u.bad:
		return fail(bridge.CodeInvalid, "The upload carried more data than it declared.")
	case u.broken:
		return fail(bridge.CodeFailed, "Could not write the upload to the disk of the PC.")
	case !u.ready || u.written != u.size || hex.EncodeToString(u.h.Sum(nil)) != u.sha:
		return fail(bridge.CodeChecksum, "The file did not arrive intact: send it again.")
	}
	if err := u.f.Close(); err != nil {
		return fail(bridge.CodeFailed, "Could not save the file.")
	}
	u.f = nil
	// Out of the map while it moves: nothing else touches it (the reader of
	// this connection is the one here).
	delete(fs.uploads, id)
	fs.upMu.Unlock()

	name := safeName(u.name)
	ext := filepath.Ext(name)
	if ext == name {
		ext = ""
	}
	stem := filesNow().Format("20060102-150405") + "-" + strings.TrimSuffix(name, ext)
	final, err := moveExclusive(filepath.Join(partials, id), inbox, stem, ext)
	os.Remove(filepath.Join(partials, id+".json"))
	if err != nil {
		os.Remove(filepath.Join(partials, id))
		return bridge.Attachment{}, "", bridge.CodeFailed, "Could not save the file."
	}
	mime := u.mime
	if mime == "" {
		mime = "application/octet-stream"
	}
	return bridge.Attachment{ID: id, Name: name, MIME: mime, Size: u.size, SHA256: u.sha}, final, "", ""
}

// finishUpload registers a filed upload, with its thumbnail, and keeps it as
// done for a retry.
func (l *phoneLink) finishUpload(att bridge.Attachment, final string) {
	l.chat.attachments.put(att, "inbox/"+final)
	l.files.upMu.Lock()
	if l.files.uploads == nil {
		l.files.uploads = map[string]*upload{}
	}
	l.files.uploads[att.ID] = &upload{sha: att.SHA256, size: att.Size, last: filesNow(), done: &att}
	l.files.upMu.Unlock()
}

// ---- downloads ----

// onFileGet sends an attachment's bytes from offset on, in frames of 256 KiB
// and then a file.end, on its own goroutine: the app's reader never waits for
// the queue of the connection.
func (l *phoneLink) onFileGet(m bridge.Message) {
	if !l.current(m) {
		return
	}
	var fg bridge.FileGet
	if json.Unmarshal(m.Payload, &fg) != nil {
		l.fail(m, bridge.CodeInvalid, "The request is not valid.")
		return
	}
	fid, err := bridge.ParseFrameID(fg.ID)
	if err != nil {
		l.fail(m, bridge.CodeInvalid, "The attachment has no valid id.")
		return
	}
	a, ok := l.chat.attachments.get(fg.ID)
	if !ok {
		l.fail(m, bridge.CodeNotFound, "There is no attachment with this id.")
		return
	}
	if fg.Offset < 0 || fg.Offset > a.Size {
		l.fail(m, bridge.CodeInvalid, "The offset is beyond the end of the file.")
		return
	}
	ws, _ := l.workspaceAndADB()
	f, err := openStored(ws, a.Path)
	if err != nil {
		l.fail(m, bridge.CodeFailed, "The file is no longer on the PC.")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &download{cancel: cancel, done: make(chan struct{})}
	l.files.dlMu.Lock()
	old := l.files.downloads[fg.ID]
	if l.files.downloads == nil {
		l.files.downloads = map[string]*download{}
	}
	l.files.downloads[fg.ID] = d
	l.files.dlMu.Unlock()
	if old != nil {
		// One download per id: the new one replaces the old, which must be
		// over before the new one sends (the app drops frames out of order).
		old.cancel()
		<-old.done
	}
	go l.runDownload(ctx, m, a, fid, fg.Offset, f, d)
}

// openStored opens the regular file of an attachment, whose path is relative
// to the environment ("inbox/…" or "outbox/…").
func openStored(ws, rel string) (*os.File, error) {
	local := filepath.FromSlash(rel)
	if ws == "" || !(strings.HasPrefix(rel, "inbox/") || strings.HasPrefix(rel, "outbox/")) || !filepath.IsLocal(local) {
		return nil, os.ErrInvalid
	}
	path := filepath.Join(ws, local)
	// A link is never followed: Lstat says what is there, and the opened file
	// must be that very one.
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() || !os.SameFile(lst, st) {
		f.Close()
		return nil, os.ErrInvalid
	}
	return f, nil
}

func (l *phoneLink) runDownload(ctx context.Context, m bridge.Message, a storedAttachment, fid bridge.FrameID, offset int64, f *os.File, d *download) {
	defer func() {
		f.Close()
		d.cancel()
		l.files.dlMu.Lock()
		if l.files.downloads[a.ID] == d {
			delete(l.files.downloads, a.ID)
		}
		l.files.dlMu.Unlock()
		close(d.done)
	}()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		l.fail(m, bridge.CodeFailed, "Could not read the file.")
		return
	}
	buf := make([]byte, bridge.MaxFrameData)
	for pos := offset; pos < a.Size; {
		n := int(min(int64(len(buf)), a.Size-pos))
		if _, err := io.ReadFull(f, buf[:n]); err != nil {
			if ctx.Err() == nil {
				l.fail(m, bridge.CodeFailed, "Could not read the file.")
			}
			return
		}
		frame, err := bridge.EncodeFrame(fid, pos, buf[:n])
		if err != nil {
			return
		}
		if err := l.srv.SendFrameTo(ctx, m, frame); err != nil {
			return // cancelled, or the app is gone
		}
		pos += int64(n)
	}
	if ctx.Err() == nil {
		_ = l.srv.Reply(m, bridge.TypeFileEnd, bridge.FileEnd{ID: a.ID, Size: a.Size})
	}
}

// onFileCancel stops the download of an id, if any.
func (l *phoneLink) onFileCancel(m bridge.Message) {
	if !l.current(m) {
		return
	}
	var fc bridge.FileCancel
	if json.Unmarshal(m.Payload, &fc) != nil {
		return
	}
	l.files.dlMu.Lock()
	d := l.files.downloads[fc.ID]
	l.files.dlMu.Unlock()
	if d != nil {
		d.cancel()
	}
}

// ---- captures ----

// onCaptureRequest takes a screenshot of the phone on its own goroutine (adb
// can take seconds and the reader answers pings), one at a time.
func (l *phoneLink) onCaptureRequest(m bridge.Message) {
	if !l.current(m) {
		return
	}
	l.files.dlMu.Lock()
	if l.files.capturing {
		l.files.dlMu.Unlock()
		l.fail(m, bridge.CodeBusy, "A screenshot is already being taken.")
		return
	}
	l.files.capturing = true
	l.files.dlMu.Unlock()
	go func() {
		defer func() {
			l.files.dlMu.Lock()
			l.files.capturing = false
			l.files.dlMu.Unlock()
		}()
		att, why := l.capture()
		if why != "" {
			l.fail(m, bridge.CodeFailed, why)
			return
		}
		_ = l.srv.Reply(m, bridge.TypeCaptureReady, bridge.CaptureReady{Attachment: att})
	}()
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// capture runs `adb exec-out screencap -p` (the person's own action: not
// through the shim) and keeps the image in inbox/. It returns the reason
// when it fails.
func (l *phoneLink) capture() (bridge.Attachment, string) {
	ws, a := l.workspaceAndADB()
	if l.Paused() {
		return bridge.Attachment{}, "Access is cut from the phone: resume it to take a screenshot."
	}
	if a == nil || ws == "" {
		return bridge.Attachment{}, "The host cannot reach the phone."
	}
	ctx, cancel := context.WithTimeout(context.Background(), captureTimeout)
	defer cancel()
	out := &limitBuffer{max: maxCaptureBytes}
	errb := &limitBuffer{max: 4 << 10}
	code, err := a.Run(ctx, "", []string{"exec-out", "screencap", "-p"}, nil, out, errb)
	switch {
	case ctx.Err() != nil:
		return bridge.Attachment{}, "The screenshot took more than 10 seconds."
	case err != nil:
		return bridge.Attachment{}, "adb failed: " + reason(err.Error())
	case out.over:
		return bridge.Attachment{}, "The screenshot is too large."
	case code != 0:
		return bridge.Attachment{}, "adb failed: " + reason(errb.String())
	}
	img := out.Bytes()
	if !bytes.HasPrefix(img, pngSignature) {
		return bridge.Attachment{}, "The phone did not return an image."
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(img)); err != nil {
		return bridge.Attachment{}, "The phone did not return a valid image."
	}

	inbox := filepath.Join(ws, "inbox")
	if err := os.MkdirAll(filepath.Join(inbox, partialDir), 0o700); err != nil {
		return bridge.Attachment{}, "Could not create the inbox folder."
	}
	l.files.upMu.Lock()
	full := inboxUsedLocked(inbox, "", l.files.uploads)+int64(len(img)) > inboxQuota
	l.files.upMu.Unlock()
	if full {
		return bridge.Attachment{}, "The inbox is full (2 GiB): free space by deleting files from the inbox folder of the environment."
	}
	f, final, err := createExclusive(inbox, filesNow().Format("20060102-150405")+"-captura", ".png")
	if err != nil {
		return bridge.Attachment{}, "Could not save the screenshot."
	}
	_, err = f.Write(img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(filepath.Join(inbox, final))
		return bridge.Attachment{}, "Could not save the screenshot."
	}
	var idb [16]byte
	if _, err := rand.Read(idb[:]); err != nil {
		os.Remove(filepath.Join(inbox, final))
		return bridge.Attachment{}, "Could not make an id."
	}
	sum := sha256.Sum256(img)
	att := bridge.Attachment{
		ID: bridge.FrameID(idb).String(), Name: "captura.png", MIME: "image/png", Size: int64(len(img)),
		SHA256: hex.EncodeToString(sum[:]), Thumb: thumbnailOf(filepath.Join(inbox, final)),
	}
	l.chat.attachments.put(att, "inbox/"+final)
	return att, ""
}

// reason makes adb's words fit a short, printable text.
func reason(s string) string {
	s = classify.Printable(strings.TrimSpace(s))
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}

// limitBuffer is a buffer that stops keeping after max bytes (over says so).
type limitBuffer struct {
	bytes.Buffer
	max  int
	over bool
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		b.over = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
