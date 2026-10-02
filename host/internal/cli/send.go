package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pathguard"
	"github.com/spf13/cobra"
)

// adbuddy send (spec M3 §5.2): the agent hands a file of its environment to
// the phone. The agent is who uses it, so unlike the session commands it is
// not refused for ADBUDDY_SOURCE=agent.

const maxSendCaption = 64 << 10 // bytes, like the text of a chat message

var (
	// maxSendSize is the biggest file that is sent, a variable for the tests.
	maxSendSize int64 = 200 << 20
	// statSize is the size a file was measured at, a variable so that a test
	// can tell the copy's own limit from the measurement's.
	statSize = func(fi os.FileInfo) int64 { return fi.Size() }
	// outboxQuota is what outbox/ may weigh (V22): the copies adbuddy send
	// leaves there and the files the agent writes there itself. The same
	// 2 GiB as inbox/ (spec M3 §6), ten copies of the biggest file.
	outboxQuota int64 = 2 << 30
	// quotaPassed runs in copyToOutbox once a send passed the quota check,
	// before its copy: a hook for the tests.
	quotaPassed = func() {}
	// readOutbox lists outbox/ to measure it: a variable so that a test can
	// make the listing fail.
	readOutbox = os.ReadDir
)

func newSendCmd(app *App) *cobra.Command {
	var caption string
	cmd := &cobra.Command{
		Use:   "send <file> [--caption \"…\"]",
		Short: app.T("help.send.short"),
		Long:  app.T("help.send.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(caption) > maxSendCaption {
				return UsageError("the caption is too large: at most 64 KiB")
			}
			path, err := filepath.Abs(args[0])
			if err != nil {
				return UsageError(err.Error())
			}
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			req := ipc.Request{Kind: ipc.KindSend, Argv: []string{path, caption}, Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}}
			exit, err := ipc.Call(cmd.Context(), e.dirs.Data, req, nil, app.Stdout, app.Stderr, nil)
			if errors.Is(err, ipc.ErrNotRunning) {
				return notRunning
			}
			if err != nil {
				return NewError("SEND_FAILED", err.Error(), "")
			}
			if exit.Code != 0 {
				return NewError(orDefault(exit.ErrorCode, "SEND_FAILED"), exit.Error, "")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&caption, "caption", "", app.T("help.send.flag.caption"))
	return cmd
}

// handleSend answers an adbuddy send: it says in the agent's terminal what
// happened. Neither the caption nor the file's content goes anywhere else.
func (l *phoneLink) handleSend(ctx context.Context, ws string, req ipc.Request, c *ipc.Conn) ipc.Exit {
	msg, exit := l.sendFile(ws, req)
	if exit.Code == 0 {
		fmt.Fprintln(c.Stdout(), msg)
	} else {
		fmt.Fprintln(c.Stderr(), exit.Error)
	}
	return exit
}

func sendFail(code int, errCode, format string, a ...any) ipc.Exit {
	return ipc.Exit{Code: code, ErrorCode: errCode, Error: fmt.Sprintf(format, a...)}
}

// sendFile checks the request, copies the file to outbox/, registers it and
// tells the app. It returns the text for the agent.
func (l *phoneLink) sendFile(ws string, req ipc.Request) (string, ipc.Exit) {
	if len(req.Argv) != 2 {
		return "", sendFail(2, "USAGE", "usage: adbuddy send <file> [--caption \"…\"]")
	}
	path, caption := req.Argv[0], req.Argv[1]
	if len(caption) > maxSendCaption {
		return "", sendFail(1, "TOO_LARGE", "the caption is too large: at most 64 KiB")
	}
	if !filepath.IsAbs(path) {
		return "", sendFail(2, "USAGE", "the path must be absolute")
	}
	if ws == "" {
		return "", sendFail(1, "SEND_FAILED", "the agent environment is unknown")
	}
	src, fi, exit := openInEnvironment(ws, path)
	if exit.Code != 0 {
		return "", exit
	}
	defer src.Close()
	if statSize(fi) > maxSendSize {
		return "", sendFail(1, "TOO_LARGE", "%s is too large: at most %d MiB", filepath.Base(path), maxSendSize>>20)
	}

	name := safeName(filepath.Base(path))
	cp, exit := l.copyToOutbox(ws, name, path, src, fi)
	if exit.Code != 0 {
		return "", exit
	}
	final, dstPath, n := cp.name, cp.path, cp.size
	// The thumbnail and the type open the copy by name: outbox/ must still be
	// a plain folder.
	if _, exit := ensureOutbox(ws); exit.Code != 0 {
		return "", exit
	}
	var idb [16]byte
	if _, err := rand.Read(idb[:]); err != nil {
		os.Remove(dstPath)
		return "", sendFail(1, "SEND_FAILED", "could not make an id")
	}
	att := bridge.Attachment{
		ID: bridge.FrameID(idb).String(), Name: name, MIME: mimeOf(name, dstPath), Size: n,
		SHA256: cp.sha256, Thumb: thumbnailOf(dstPath),
	}
	l.chat.attachments.put(att, "outbox/"+final)

	// Without the chat mutex: the session answers to its own lock.
	conv := ""
	if m := l.session(); m != nil {
		conv = m.Info().Conversation
	}
	if conv == "" {
		l.chat.mu.Lock()
		conv = l.chat.conv
		l.chat.mu.Unlock()
	}
	l.addMessage(bridge.ChatMessage{ID: "send_" + hex.EncodeToString(idb[:8]), Role: "assistant", Text: caption, Attachments: []bridge.Attachment{att}, Conversation: conv})

	msg := fmt.Sprintf("Sent %s (%d bytes) to the phone.", name, n)
	if l.srv == nil || !l.srv.Connected() {
		msg = fmt.Sprintf("Saved %s (%d bytes) for the phone: the app is not connected, it will get it when it connects.", name, n)
	}
	return msg, ipc.Exit{}
}

// outboxCopy is the copy of a file that adbuddy send left in outbox/.
type outboxCopy struct {
	name   string // in outbox/
	path   string // outbox/name, absolute
	size   int64
	sha256 string // hex
}

// copyToOutbox copies src (the file at path, measured as fi) into outbox/ of
// ws under a new name made from name, within the quota of outbox/ (V22).
// Sends take turns here, from the check to the end of the copy (sendMu): two
// at once could both fit in the room that only one of them has. The turn is
// not held while the session or the bridge server are called.
func (l *phoneLink) copyToOutbox(ws, name, path string, src *os.File, fi os.FileInfo) (outboxCopy, ipc.Exit) {
	l.files.sendMu.Lock()
	defer l.files.sendMu.Unlock()
	outbox, exit := ensureOutbox(ws)
	if exit.Code != 0 {
		return outboxCopy{}, exit
	}
	used, err := outboxUsed(outbox)
	if err != nil {
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not measure the outbox folder")
	}
	if used+statSize(fi) > outboxQuota {
		return outboxCopy{}, outboxFull()
	}
	quotaPassed()
	ext := filepath.Ext(name)
	stem := filesNow().Format("20060102-150405") + "-" + strings.TrimSuffix(name, ext)
	dst, final, err := createExclusive(outbox, stem, ext)
	if err != nil {
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not save the file in outbox/")
	}
	dstPath := filepath.Join(outbox, final)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, maxSendSize+1))
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		os.Remove(dstPath)
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not copy the file")
	case n > maxSendSize:
		os.Remove(dstPath)
		return outboxCopy{}, sendFail(1, "TOO_LARGE", "%s is too large: at most %d MiB", filepath.Base(path), maxSendSize>>20)
	}
	used, err = outboxUsed(outbox) // it may have grown after it was measured
	switch {
	case err != nil:
		os.Remove(dstPath)
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not measure the outbox folder")
	case used > outboxQuota:
		os.Remove(dstPath)
		return outboxCopy{}, outboxFull()
	}
	return outboxCopy{name: final, path: dstPath, size: n, sha256: hex.EncodeToString(h.Sum(nil))}, ipc.Exit{}
}

// outboxUsed is what outbox/ weighs for its quota: the regular files at its
// top, where adbuddy send leaves its copies. The agent's own files there
// count too; its subfolders are its own, like the rest of the environment.
// A folder that cannot be listed is an error, not an empty one: the quota
// fails closed.
func outboxUsed(outbox string) (int64, error) {
	var total int64
	entries, err := readOutbox(outbox)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total, nil
}

// outboxFull is the answer to a send that does not fit in outbox/.
func outboxFull() ipc.Exit {
	return sendFail(1, "TOO_LARGE", "the outbox folder is full (at most %d MiB): delete from it the files the phone already has", outboxQuota>>20)
}

// openInEnvironment opens the regular file at path if it is inside the
// environment once links are followed, as the daemon does for adb pull:
// links are resolved, the comparison ignores case on Windows and a junction
// (which EvalSymlinks does not follow) is refused. The file opened must be
// the one resolved, and it is never opened before it is known to be regular
// (a FIFO would block open, and with it the shutdown of serve).
func openInEnvironment(ws, path string) (*os.File, os.FileInfo, ipc.Exit) {
	outside := sendFail(1, "OUTSIDE_WORKSPACE", "%s is outside the agent environment: only files inside it can be sent", classify.Printable(path))
	root, err := filepath.EvalSymlinks(ws)
	if err != nil {
		return nil, nil, sendFail(1, "SEND_FAILED", "the agent environment cannot be read")
	}
	if path != filepath.Clean(path) {
		path = filepath.Clean(path)
	}
	real, err := pathguard.ResolveLinks(path)
	if err != nil {
		// Only a link that does not resolve (a missing file resolves to
		// where it would be: Lstat tells below).
		return nil, nil, outside
	}
	rel, inside := pathguard.Within(root, real)
	if !inside || pathguard.ReparseBelow(root, rel) {
		return nil, nil, outside
	}
	lst, err := os.Lstat(real)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, sendFail(1, "NOT_FOUND", "%s does not exist", classify.Printable(path))
		}
		return nil, nil, sendFail(1, "SEND_FAILED", "could not read %s", classify.Printable(path))
	}
	if !lst.Mode().IsRegular() {
		return nil, nil, sendFail(1, "NOT_A_FILE", "%s is not a regular file: only files can be sent", classify.Printable(path))
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, nil, sendFail(1, "SEND_FAILED", "could not open %s", classify.Printable(path))
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || !os.SameFile(lst, st) {
		f.Close()
		return nil, nil, sendFail(1, "SEND_FAILED", "%s changed while it was being opened", classify.Printable(path))
	}
	afterOpen()
	// SameFile does not see a folder on the way that became a link after the
	// first resolution: resolve again, it must be the same path.
	again, err := resolveAgain(real)
	if err != nil || !samePath(again, real) {
		f.Close()
		return nil, nil, outside
	}
	return f, st, ipc.Exit{}
}

// ensureOutbox creates outbox/ of the environment and checks that it is a
// real folder: the agent can replace it with a link, and the copy would be
// written wherever it points.
func ensureOutbox(ws string) (string, ipc.Exit) {
	outbox := filepath.Join(ws, "outbox")
	if err := os.MkdirAll(outbox, 0o700); err != nil {
		return "", sendFail(1, "SEND_FAILED", "could not create the outbox folder")
	}
	if !plainDir(outbox) {
		return "", sendFail(1, "SEND_FAILED", "outbox in the environment is not a plain folder")
	}
	return outbox, ipc.Exit{}
}

// plainDir reports whether p is a real folder: not a symlink, a junction or
// another kind of link.
func plainDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir() && fi.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// afterOpen runs between opening the file and resolving its path again: a
// hook for the tests.
var afterOpen = func() {}

// resolveAgain is the second resolution of the path, a variable for the tests.
var resolveAgain = pathguard.ResolveLinks

// mimeOf is the type of a file by its extension, else by its first bytes.
func mimeOf(name, path string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		t, _, _ = strings.Cut(t, ";")
		return t
	}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		buf := make([]byte, 512)
		n, _ := io.ReadFull(f, buf)
		t, _, _ := strings.Cut(http.DetectContentType(buf[:n]), ";")
		return t
	}
	return "application/octet-stream"
}
