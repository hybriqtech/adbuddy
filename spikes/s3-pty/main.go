// Spike S3 de ADBuddy (código desechable): comprueba si Claude Code funciona dentro
// de un pseudoterminal ConPTY controlado desde Go en Windows.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/UserExistsError/conpty"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// ansiRe quita las secuencias CSI, OSC y de juego de caracteres para buscar texto plano.
var ansiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78]`)

type output struct {
	mu   sync.Mutex
	raw  strings.Builder
	last time.Time
}

func (o *output) Write(p []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.raw.Write(p)
	o.last = time.Now()
}

func (o *output) Len() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.raw.Len()
}

func (o *output) RawFrom(offset int) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := o.raw.String()
	if offset > len(s) {
		offset = len(s)
	}
	return s[offset:]
}

func (o *output) PlainFrom(offset int) string {
	return ansiRe.ReplaceAllString(o.RawFrom(offset), "")
}

func (o *output) Idle() time.Duration {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.last.IsZero() {
		return 0
	}
	return time.Since(o.last)
}

func waitUntil(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return cond()
}

type spike struct {
	cpty *conpty.ConPty
	out  *output
}

func (s *spike) send(text string) {
	if _, err := s.cpty.Write([]byte(text)); err != nil {
		fmt.Fprintf(os.Stderr, "error al escribir: %v\n", err)
	}
}

// submit pega el texto con bracketed paste y pulsa Enter, igual que hará el host de ADBuddy.
func (s *spike) submit(text string) {
	s.send(pasteStart + text + pasteEnd)
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
}

// settle espera a que la salida lleve 3 s quieta (60 s como máximo).
func (s *spike) settle() {
	waitUntil(60*time.Second, func() bool { return s.out.Idle() > 3*time.Second })
}

// ask envía un prompt y comprueba que la respuesta contiene want (que no aparece en el prompt).
func (s *spike) ask(name, prompt, want string) {
	offset := s.out.Len()
	s.submit(prompt)
	ok := waitUntil(180*time.Second, func() bool { return strings.Contains(s.out.PlainFrom(offset), want) })
	report(name, ok, fmt.Sprintf("esperado %q", want))
	s.settle()
}

func report(name string, ok bool, detail string) {
	status := "FAIL"
	if ok {
		status = "PASS"
	}
	fmt.Printf("| %s | %s | %s |\n", name, status, detail)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func main() {
	claude := flag.String("claude", `C:\Users\Jose\.local\bin\claude.exe`, "ruta de claude.exe")
	workDir := flag.String("dir", "work", "carpeta de trabajo de la sesión")
	outDir := flag.String("out", "out", "carpeta para los logs")
	flag.Parse()

	absWork, err := filepath.Abs(*workDir)
	must(err)
	must(os.MkdirAll(*outDir, 0o755))
	if !conpty.IsConPtyAvailable() {
		fmt.Println("ConPTY no está disponible en este Windows")
		os.Exit(1)
	}

	cpty, err := conpty.Start(*claude, conpty.ConPtyDimensions(120, 40), conpty.ConPtyWorkDir(absWork))
	must(err)
	defer cpty.Close()

	out := &output{}
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := cpty.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	s := &spike{cpty: cpty, out: out}

	fmt.Println("| Check | Resultado | Detalle |")
	fmt.Println("|---|---|---|")

	// C1: arranque y posible diálogo de confianza de la carpeta.
	started := waitUntil(45*time.Second, func() bool { return out.Len() > 0 && out.Idle() > 2*time.Second })
	trust := strings.Contains(strings.ToLower(out.PlainFrom(0)), "trust")
	if trust {
		// Desviación tras la 1.ª ejecución: la opción por defecto es "No, exit";
		// se baja a "Yes, I trust this folder" antes de confirmar.
		s.send("\x1b[B")
		time.Sleep(300 * time.Millisecond)
		s.send("\r")
		s.settle()
	}
	report("C1 arranque", started, fmt.Sprintf("%d bytes; diálogo de confianza: %v", out.Len(), trust))

	// C2: ¿activa Claude Code el modo bracketed paste?
	report("C2 bracketed paste activo", strings.Contains(out.RawFrom(0), "\x1b[?2004h"), "busca ESC[?2004h")

	// C3: una línea con Unicode en la entrada y en la salida.
	s.ask("C3 una línea + Unicode",
		"Responde únicamente con la palabra ñandú en mayúsculas seguida del resultado de 6*7, sin espacios ni nada más.",
		"ÑANDÚ42")

	// C4: varias líneas pegadas de una vez deben llegar como un solo mensaje.
	s.ask("C4 multilínea",
		"Cuenta las líneas de este mensaje.\nEsta es la segunda línea.\nY esta es la última. Responde solo con el número, escrito con letras mayúsculas.",
		"TRES")

	// C5: redimensionar y seguir funcionando.
	must(cpty.Resize(80, 30))
	s.settle()
	s.ask("C5 tras redimensionar a 80x30",
		"Responde únicamente con el color del cielo despejado, en español y en mayúsculas.",
		"AZUL")

	// C6: Esc interrumpe una respuesta larga.
	offset := out.Len()
	s.submit("Escribe los números del 1 al 2000, uno por línea, sin nada más.")
	time.Sleep(6 * time.Second)
	s.send("\x1b")
	interrupted := waitUntil(20*time.Second, func() bool {
		return strings.Contains(strings.ToLower(out.PlainFrom(offset)), "interrupted")
	})
	report("C6 Esc interrumpe", interrupted, "busca 'Interrupted' tras enviar Esc")
	s.settle()

	// C7: colores en la salida (SGR de 256 colores o truecolor).
	raw := out.RawFrom(0)
	report("C7 colores", strings.Contains(raw, "\x1b[38;5;") || strings.Contains(raw, "\x1b[38;2;"), "busca ESC[38;5; o ESC[38;2;")

	// C8: /exit termina el proceso limpiamente.
	s.send("/exit")
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, werr := cpty.Wait(ctx)
	report("C8 /exit termina", werr == nil, fmt.Sprintf("código %d, error %v", code, werr))

	must(os.WriteFile(filepath.Join(*outDir, "raw.log"), []byte(out.RawFrom(0)), 0o644))
	must(os.WriteFile(filepath.Join(*outDir, "plain.log"), []byte(out.PlainFrom(0)), 0o644))
}
