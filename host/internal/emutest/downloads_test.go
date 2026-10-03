package emutest

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	mrand "math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// Files over the channel, the downloads (validation 3 of M3b, c-e): the agent
// sends a file in a chat.message, the app asks for it with file.get and the
// host answers with binary frames and file.end. Mutations are listed in
// files_test.go (ADBUDDY_EMUTEST_MUTATE).

// hostFile is a file the host has: what it announces and its real bytes.
type hostFile struct {
	att  bridge.Attachment
	data []byte
}

// newHostFile makes the attachment of data: a random id (32 hex digits, as
// the host's), its size and the SHA-256 of the bytes.
func newHostFile(t *testing.T, name, mime string, data []byte) hostFile {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hostFile{data: data, att: bridge.Attachment{
		ID: hex.EncodeToString(raw[:]), Name: name, MIME: mime, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}}
}

// noise returns n pseudo-random bytes (the same ones for the same seed).
func noise(n int, seed byte) []byte {
	b := make([]byte, n)
	mrand.NewChaCha8([32]byte{seed, 'd', 'o', 'w', 'n'}).Read(b)
	return b
}

// pngFile is a valid PNG of noise, about 1.4 MB: under the 5 MiB below which
// the app downloads an image by itself.
func pngFile(t *testing.T, name string, seed byte) hostFile {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 600, 600))
	copy(img.Pix, noise(len(img.Pix), seed))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("the generated PNG is not valid: %v", err)
	}
	if buf.Len() > 5<<20 {
		t.Fatalf("the PNG has %d bytes: over the 5 MiB of the automatic download", buf.Len())
	}
	return newHostFile(t, name, "image/png", buf.Bytes())
}

// pdfFile is size bytes that start and end as a PDF.
func pdfFile(t *testing.T, name string, size int, seed byte) hostFile {
	t.Helper()
	const head, tail = "%PDF-1.4\n", "\n%%EOF\n"
	data := append([]byte(head), noise(size-len(head)-len(tail), seed)...)
	return newHostFile(t, name, "application/pdf", append(data, tail...))
}

// say publishes an assistant message with the file attached (one with an id
// already seen replaces it, so it can be published again after a resume).
func (l *link) say(t *testing.T, msgID string, f hostFile) bridge.ChatMessage {
	t.Helper()
	cm := bridge.ChatMessage{ID: msgID, Role: "assistant", Text: "here is " + f.att.Name, Attachments: []bridge.Attachment{f.att}}
	l.send(t, bridge.TypeChatMessage, cm)
	return cm
}

// nextGet waits up to d for the app's next file.get and returns it with its
// request.
func (l *link) nextGet(t *testing.T, d time.Duration) (bridge.FileGet, bridge.Message) {
	t.Helper()
	m := l.waitFor(t, bridge.TypeFileGet, d)
	return payload[bridge.FileGet](t, m), m
}

// noGet fails when a file.get comes in the next d.
func (l *link) noGet(t *testing.T, what string, d time.Duration) {
	t.Helper()
	if m, ok := l.take(bridge.TypeFileGet, d); ok {
		t.Errorf("a file.get %s: %s", what, m.Payload)
	}
}

// tapForGet taps the card that says text and returns the file.get it brings.
// A tap right after the card appears can be lost (the list is still
// recomposing), so it is repeated, up to 3 times, while no file.get comes in
// about 3 s.
func (l *link) tapForGet(t *testing.T, text string) (bridge.FileGet, bridge.Message) {
	t.Helper()
	e := l.e
	card := e.waitText(t, text, 10*time.Second)
	for try := 1; try <= 3; try++ {
		e.tap(t, card)
		if m, ok := l.take(bridge.TypeFileGet, 3*time.Second); ok {
			return payload[bridge.FileGet](t, m), m
		}
		card = e.waitText(t, text, 10*time.Second)
	}
	t.Fatalf("no file.get after 3 taps on %q", text)
	return bridge.FileGet{}, bridge.Message{}
}

// runAs runs command as the app (run-as: the debug build allows it).
func (e *emu) runAs(t *testing.T, command string) string {
	t.Helper()
	return e.shell(t, "run-as "+phoneapp.Package+" "+command)
}

// cacheLs lists the app's cache/downloads: name -> size in bytes.
func (e *emu) cacheLs(t *testing.T) map[string]int64 {
	t.Helper()
	out := e.shell(t, "run-as "+phoneapp.Package+" ls -l cache/downloads; true")
	files := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || !strings.HasPrefix(f[0], "-") {
			continue
		}
		n, err := strconv.ParseInt(f[4], 10, 64)
		if err != nil {
			t.Fatalf("ls -l line %q: %v", line, err)
		}
		files[f[len(f)-1]] = n
	}
	return files
}

// cacheSHA is the SHA-256 the emulator computes of cache/downloads/name.
func (e *emu) cacheSHA(t *testing.T, name string) string {
	t.Helper()
	f := strings.Fields(e.runAs(t, "sha256sum cache/downloads/"+name))
	if len(f) != 2 {
		t.Fatalf("sha256sum of %s: %q", name, f)
	}
	return f[0]
}

// partSize is the size of the id's .part in the cache (-1: there is none).
func (e *emu) partSize(t *testing.T, id string) int64 {
	t.Helper()
	if n, ok := e.cacheLs(t)[id+".part"]; ok {
		return n
	}
	return -1
}

// countText counts the nodes on screen whose text is text.
func countText(nodes []uiNode, text string) int {
	n := 0
	for _, x := range nodes {
		if x.Text == text {
			n++
		}
	}
	return n
}

// waitText waits for a node with exactly that text.
func (e *emu) waitText(t *testing.T, text string, d time.Duration) uiNode {
	t.Helper()
	return e.waitNode(t, text, d, func(n uiNode) bool { return n.Text == text })
}

// waitCount waits until exactly n nodes show text.
func (e *emu) waitCount(t *testing.T, text string, n int, d time.Duration) {
	t.Helper()
	var last []uiNode
	if !poll(d, func() bool {
		last = e.screen(t)
		return countText(last, text) == n
	}) {
		t.Fatalf("not %d nodes %q on screen in %d s: %+v", n, text, int(d/time.Second), last)
	}
}

// cacheIs checks that cache/downloads holds name with exactly the SHA-256 and
// size of f (hash computed by the emulator on the file in the cache).
func (e *emu) cacheIs(t *testing.T, f hostFile) {
	t.Helper()
	name := f.att.ID + "-" + f.att.Name
	if got, ok := e.cacheLs(t)[name]; !ok || got != f.att.Size {
		t.Errorf("cache/downloads/%s: size %d (present %v), want %d", name, got, ok, f.att.Size)
		return
	}
	if got := e.cacheSHA(t, name); got != f.att.SHA256 {
		t.Errorf("sha256 of cache/downloads/%s is %s, the host's file has %s", name, got, f.att.SHA256)
	}
}

// Validation 3 of M3b (c): an image the agent sends downloads by itself (one
// file.get from 0, under 5 MiB), a PDF only when tapped; the app writes the
// bytes the host sent and the file in the cache has the host's SHA-256,
// computed by the emulator on the file itself.
func TestDownloadsBringTheRealBytes(t *testing.T) {
	mutate := mutation(t, "corrupt-frame")
	l := openLink(t)
	e := l.e
	l.openChat(t)
	opt := serveOpts{corrupt: mutate == "corrupt-frame"}
	img := pngFile(t, "pic.png", 1)
	pdf := pdfFile(t, "big.pdf", 6<<20, 2)

	// The image: nobody taps, the app asks for it.
	l.drain()
	l.say(t, "d1", img)
	g, req := l.nextGet(t, 15*time.Second)
	if g.ID != img.att.ID || g.Offset != 0 {
		t.Fatalf("the automatic download asked %+v, want %s from 0", g, img.att.ID)
	}
	l.serveFile(t, req, img.data, opt)
	e.waitText(t, "Tap to open", 15*time.Second)
	l.noGet(t, "after the image was downloaded", 2*time.Second)
	e.cacheIs(t, img)

	// The PDF: it waits for the tap.
	l.say(t, "d2", pdf)
	card := e.waitText(t, "Tap to download", 10*time.Second)
	l.noGet(t, "for a PDF nobody tapped", 3*time.Second)
	e.tap(t, card)
	g, req = l.nextGet(t, 10*time.Second)
	if g.ID != pdf.att.ID || g.Offset != 0 {
		t.Fatalf("the tap asked %+v, want %s from 0", g, pdf.att.ID)
	}
	l.serveFile(t, req, pdf.data, opt)
	e.waitCount(t, "Tap to open", 2, 15*time.Second)
	l.noGet(t, "after the PDF was downloaded", 2*time.Second)
	e.cacheIs(t, pdf)
	if p := e.partSize(t, pdf.att.ID); p != -1 {
		t.Errorf("a .part of %d bytes is left after the download finished", p)
	}
}

// Validation 3 of M3b (c), the cut: the channel drops in the middle of a
// download and the app keeps what it had in the .part. After the reconnection
// the chat is empty (the new server has no history, T1), the host publishes
// the message again and the person taps the card: the app asks for the file
// from exactly the size of the .part (the emulator's stat, not what the host
// sent) and only for the rest, once, and the file ends with the host's hash.
func TestDownloadResumesAfterACut(t *testing.T) {
	mutate := mutation(t, "corrupt-frame", "ignore-offset")
	l := openLink(t)
	e := l.e
	l.openChat(t)
	const stall = 2_000_000
	pdf := pdfFile(t, "big.pdf", 6<<20, 3)

	l.drain()
	cm := l.say(t, "r1", pdf)
	g, req := l.tapForGet(t, "Tap to download")
	if g.ID != pdf.att.ID || g.Offset != 0 {
		t.Fatalf("the tap asked %+v, want %s from 0", g, pdf.att.ID)
	}
	// The host stalls after 2 MB: no file.end, the app waits.
	l.serveFile(t, req, pdf.data, serveOpts{limit: stall, corrupt: mutate == "corrupt-frame"})
	e.waitNode(t, "the download running", 10*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, "Downloading") })
	if !poll(20*time.Second, func() bool { return e.partSize(t, pdf.att.ID) == stall }) {
		t.Fatalf(".part has %d bytes, the host sent %d", e.partSize(t, pdf.att.ID), stall)
	}

	hellos := len(l.Hellos())
	l.cut()
	e.waitText(t, "Download failed: tap to retry", 40*time.Second)
	have := e.partSize(t, pdf.att.ID)
	if have != stall {
		t.Fatalf("after the cut the .part has %d bytes, want the %d it had", have, stall)
	}
	l.resume(t)
	l.waitHellos(t, hellos+1, 60*time.Second)
	l.drain()
	// What a real host does when the app comes back: the message again.
	l.send(t, bridge.TypeChatMessage, cm)
	e.tap(t, e.waitText(t, "Download failed: tap to retry", 20*time.Second))

	g, req = l.nextGet(t, 10*time.Second)
	if g.ID != pdf.att.ID || g.Offset != have || g.Offset <= 0 {
		t.Fatalf("after the cut the app asked %+v, want %s from %d (the size of the .part, > 0)", g, pdf.att.ID, have)
	}
	l.serveFile(t, req, pdf.data, serveOpts{fromZero: mutate == "ignore-offset", corrupt: mutate == "corrupt-frame"})
	// Exactly one request: a second one means the file did not end well (it
	// is asked again from 0 when the hash is wrong).
	if extra, ok := l.take(bridge.TypeFileGet, 5*time.Second); ok {
		t.Errorf("a second file.get after the resumed one: %s", extra.Payload)
		l.serveFile(t, extra, pdf.data, serveOpts{})
	}
	e.waitText(t, "Tap to open", 20*time.Second)
	e.cacheIs(t, pdf)
	if p := e.partSize(t, pdf.att.ID); p != -1 {
		t.Errorf("a .part of %d bytes is left after the download finished", p)
	}
}

// Validation 3 of M3b (c), a wrong hash: the app asks once more from 0, and if
// the bytes are wrong again it fails and leaves neither the .part nor the file.
// (A file.end with a wrong Size breaks nothing: the app checks the length of
// what it wrote and the SHA-256 of the attachment, never that Size; the last
// step shows it.)
func TestDownloadRetriesOnceThenFails(t *testing.T) {
	mutate := mutation(t, "heal-on-retry")
	l := openLink(t)
	e := l.e
	l.openChat(t)
	pdf := pdfFile(t, "bad.pdf", 1<<20, 4)

	l.drain()
	l.say(t, "w1", pdf)
	g, req := l.tapForGet(t, "Tap to download")
	if g.ID != pdf.att.ID || g.Offset != 0 {
		t.Fatalf("the tap asked %+v, want %s from 0", g, pdf.att.ID)
	}
	l.serveFile(t, req, pdf.data, serveOpts{corrupt: true})
	// The wrong hash: asked once more, from 0.
	g, req = l.nextGet(t, 15*time.Second)
	if g.ID != pdf.att.ID || g.Offset != 0 {
		t.Fatalf("after the wrong hash the app asked %+v, want %s again from 0", g, pdf.att.ID)
	}
	l.serveFile(t, req, pdf.data, serveOpts{corrupt: mutate != "heal-on-retry"})
	e.waitText(t, "Download failed: tap to retry", 15*time.Second)
	l.noGet(t, "after the second wrong hash (it is asked once more, no more)", 4*time.Second)
	files := e.cacheLs(t)
	for name := range files {
		if strings.HasPrefix(name, pdf.att.ID) {
			t.Errorf("cache/downloads/%s is left after the failed download", name)
		}
	}

	// file.end with a wrong Size: nothing breaks.
	e.tap(t, e.waitText(t, "Download failed: tap to retry", 5*time.Second))
	g, req = l.nextGet(t, 10*time.Second)
	if g.Offset != 0 {
		t.Fatalf("the retry tap asked %+v, want from 0", g)
	}
	l.serveFile(t, req, pdf.data, serveOpts{endSize: int64(len(pdf.data)) + 12345})
	e.waitText(t, "Tap to open", 15*time.Second)
	e.cacheIs(t, pdf)
}

// Validation 3 of M3b (c), cancel: tapping a download in progress sends
// file.cancel; the .part stops where it was (frames that still arrive are
// dropped, even ones at the right offset) and the card offers the download
// again. The .part stays, to be resumed (T1).
func TestDownloadCancel(t *testing.T) {
	l := openLink(t)
	e := l.e
	l.openChat(t)
	const stall = 1_500_000
	pdf := pdfFile(t, "cancel.pdf", 6<<20, 5)

	l.drain()
	l.say(t, "c1", pdf)
	_, req := l.tapForGet(t, "Tap to download")
	l.serveFile(t, req, pdf.data, serveOpts{limit: stall})
	running := e.waitNode(t, "the download running", 10*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, "Downloading") })
	if !poll(20*time.Second, func() bool { return e.partSize(t, pdf.att.ID) == stall }) {
		t.Fatalf(".part has %d bytes, the host sent %d", e.partSize(t, pdf.att.ID), stall)
	}

	e.tap(t, running)
	m := l.waitFor(t, bridge.TypeFileCancel, 10*time.Second)
	if c := payload[bridge.FileCancel](t, m); c.ID != pdf.att.ID {
		t.Errorf("file.cancel for %s, the download is %s", c.ID, pdf.att.ID)
	}
	e.waitText(t, "Tap to download", 10*time.Second)
	// The host did not hear in time: it sends the next frame, right at the
	// offset the .part is at. The app must drop it.
	late := pdf.data[stall : stall+bridge.MaxFrameData]
	id, err := bridge.ParseFrameID(pdf.att.ID)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := bridge.EncodeFrame(id, stall, late)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.srv.SendFrameTo(e.ctx, req, frame); err != nil {
		t.Fatalf("send the late frame: %v", err)
	}
	time.Sleep(2 * time.Second)
	if p := e.partSize(t, pdf.att.ID); p != stall {
		t.Errorf("after the cancel the .part has %d bytes, want the %d it had (it must not grow)", p, stall)
	}
	l.noGet(t, "after the cancel", 2*time.Second)
	if slices.Contains(mapKeys(e.cacheLs(t)), pdf.att.ID+"-"+pdf.att.Name) {
		t.Errorf("a finished file is in the cache after a cancelled download")
	}
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	var ks []K
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
